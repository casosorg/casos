import React, {useEffect, useRef, useState} from "react";
import {CheckCircle2, Clock, LoaderCircle, Lock, TriangleAlert} from "lucide-react";
import * as CertificateBackend from "@/backend/CertificateBackend";
import * as Setting from "@/Setting";
import {Alert, AlertDescription, AlertTitle} from "@/components/ui/alert";
import {Button} from "@/components/ui/button";
import {Dialog, DialogContent, DialogHeader, DialogTitle} from "@/components/ui/dialog";
import {Input} from "@/components/ui/input";
import {Tabs, TabsContent, TabsList, TabsTrigger} from "@/components/ui/tabs";
import {Textarea} from "@/components/ui/textarea";
import {Field} from "@/components/shared/form-dialog";
import {NumberInput} from "@/components/shared/number-input";
import {CodeText} from "@/components/shared/misc";

const POLL_INTERVAL = 4000;

const STATUS_PRESENTATION = {
  issued: {variant: "success", Icon: CheckCircle2},
  verifying: {variant: "info", Icon: LoaderCircle},
  pending: {variant: "info", Icon: LoaderCircle},
  failed: {variant: "destructive", Icon: TriangleAlert},
  none: {variant: "default", Icon: Clock},
};

function statusLabel(certStatus) {
  switch (certStatus.status) {
  case "issued":
    return `Certificate active — expires ${certStatus.expiry ?? "unknown"}`;
  case "verifying":
    return "Verifying domain ownership with Let's Encrypt…";
  case "pending":
    return "Certificate request queued…";
  case "failed":
    return `Failed: ${certStatus.error ?? "unknown error"}`;
  default:
    return certStatus.status;
  }
}

/**
 * HTTPS for one Ingress, either by asking Let's Encrypt for a certificate or by
 * pasting one in. An in-flight ACME request is polled until it settles, because
 * issuance is asynchronous and can take a couple of minutes.
 */
export function CertificateDialog({ingress, open, onClose, onUpdated}) {
  const [tab, setTab] = useState("le");
  const [submitting, setSubmitting] = useState(false);
  const [certStatus, setCertStatus] = useState(null);
  const [leForm, setLeForm] = useState({domain: "", casosServiceName: "", casosServicePort: ""});
  const [uploadForm, setUploadForm] = useState({certPEM: "", keyPEM: ""});
  const [errors, setErrors] = useState({});
  const pollTimer = useRef(null);

  const domain = (ingress?.rules ?? [])[0]?.host ?? "";

  function stopPolling() {
    if (pollTimer.current) {
      clearInterval(pollTimer.current);
      pollTimer.current = null;
    }
  }

  useEffect(() => stopPolling, []);

  useEffect(() => {
    if (!open || !ingress) {
      stopPolling();
      return;
    }
    setTab("le");
    setErrors({});
    setUploadForm({certPEM: "", keyPEM: ""});
    setLeForm({domain, casosServiceName: "", casosServicePort: ""});

    let cancelled = false;

    function fetchStatus() {
      CertificateBackend.getCertStatus(ingress.namespace, ingress.name)
        .then((res) => {
          if (cancelled || res.status !== "ok") {
            return;
          }
          setCertStatus(res.data);
          if (res.data?.status !== "pending" && res.data?.status !== "verifying") {
            stopPolling();
          } else if (!pollTimer.current) {
            pollTimer.current = setInterval(fetchStatus, POLL_INTERVAL);
          }
        })
        .catch(() => {});
    }

    fetchStatus();
    return () => {
      cancelled = true;
      stopPolling();
    };
  }, [open, ingress, domain]);

  function handleRequestLE() {
    if (!leForm.domain) {
      setErrors({domain: "Domain is required"});
      return;
    }
    setErrors({});
    setSubmitting(true);
    CertificateBackend.requestLECert({
      namespace: ingress.namespace,
      ingressName: ingress.name,
      domain: leForm.domain,
      casosServiceName: leForm.casosServiceName || undefined,
      casosServicePort: leForm.casosServicePort || undefined,
    })
      .then((res) => {
        if (res.status === "ok") {
          Setting.showMessage("success", "Certificate request started — this may take up to 2 minutes");
          setCertStatus({status: "pending"});
          if (!pollTimer.current) {
            pollTimer.current = setInterval(() => {
              CertificateBackend.getCertStatus(ingress.namespace, ingress.name)
                .then((statusRes) => {
                  if (statusRes.status !== "ok") {
                    return;
                  }
                  setCertStatus(statusRes.data);
                  if (statusRes.data?.status !== "pending" && statusRes.data?.status !== "verifying") {
                    stopPolling();
                    onUpdated?.();
                  }
                })
                .catch(() => {});
            }, POLL_INTERVAL);
          }
        } else {
          Setting.showMessage("error", res.msg);
        }
      })
      .catch((error) => Setting.showMessage("error", error.message))
      .finally(() => setSubmitting(false));
  }

  function handleUpload() {
    const nextErrors = {};
    if (!uploadForm.certPEM.trim().includes("-----BEGIN CERTIFICATE-----")) {
      nextErrors.certPEM = "Must be a valid PEM certificate";
    }
    if (!uploadForm.keyPEM.trim().includes("PRIVATE KEY")) {
      nextErrors.keyPEM = "Must be a valid PEM private key";
    }
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    setSubmitting(true);
    CertificateBackend.uploadCert({
      namespace: ingress.namespace,
      ingressName: ingress.name,
      certPEM: uploadForm.certPEM.trim(),
      keyPEM: uploadForm.keyPEM.trim(),
    })
      .then((res) => {
        if (res.status === "ok") {
          Setting.showMessage("success", "Certificate uploaded and applied");
          setCertStatus(res.data);
          onUpdated?.();
        } else {
          Setting.showMessage("error", res.msg);
        }
      })
      .catch((error) => Setting.showMessage("error", error.message))
      .finally(() => setSubmitting(false));
  }

  const presentation = certStatus ? STATUS_PRESENTATION[certStatus.status] ?? STATUS_PRESENTATION.none : null;
  const inFlight = certStatus?.status === "pending" || certStatus?.status === "verifying";

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          stopPolling();
          onClose();
        }
      }}
    >
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Lock className="text-info size-4" />
            Manage HTTPS — <CodeText>{ingress?.name ?? ""}</CodeText>
          </DialogTitle>
        </DialogHeader>

        {certStatus && certStatus.status !== "none" && presentation ? (
          <Alert variant={presentation.variant}>
            <presentation.Icon className={inFlight ? "animate-spin" : undefined} />
            <AlertTitle>{statusLabel(certStatus)}</AlertTitle>
          </Alert>
        ) : null}

        <Tabs value={tab} onValueChange={setTab}>
          <TabsList className="w-full">
            <TabsTrigger value="le">Let&apos;s Encrypt (auto)</TabsTrigger>
            <TabsTrigger value="upload">Upload Certificate</TabsTrigger>
          </TabsList>

          <TabsContent value="le" className="grid gap-4 pt-4">
            <Alert variant="info">
              <AlertTitle>Requirements</AlertTitle>
              <AlertDescription>
                <ul className="list-disc pl-4">
                  <li>The domain must be publicly reachable on port 80 through your Ingress controller.</li>
                  <li>The cluster must be able to reach the casos server; the Service the challenge routes to is created for you.</li>
                </ul>
              </AlertDescription>
            </Alert>

            <Field
              label="Domain"
              htmlFor="cert-domain"
              required
              error={errors.domain}
              hint="The public domain to issue the certificate for."
            >
              <Input
                id="cert-domain"
                value={leForm.domain}
                onChange={(event) => setLeForm((prev) => ({...prev, domain: event.target.value}))}
                placeholder="myapp.example.com"
              />
            </Field>

            <Field
              label="casos Service Name"
              htmlFor="cert-service"
              hint="Kubernetes Service exposing the casos server, created when it does not exist. Leave blank to use the value from app.conf."
            >
              <Input
                id="cert-service"
                value={leForm.casosServiceName}
                onChange={(event) => setLeForm((prev) => ({...prev, casosServiceName: event.target.value}))}
                placeholder="casos"
              />
            </Field>

            <Field label="casos Service Port" hint="Port of the casos Service. Leave blank to use the port casos serves on.">
              <NumberInput
                value={leForm.casosServicePort}
                onChange={(next) => setLeForm((prev) => ({...prev, casosServicePort: next}))}
                min={1}
                max={65535}
              />
            </Field>

            <div>
              <Button onClick={handleRequestLE} loading={submitting} disabled={inFlight}>
                <Lock />
                Request Free Certificate
              </Button>
            </div>
          </TabsContent>

          <TabsContent value="upload" className="grid gap-4 pt-4">
            <Field label="Certificate (PEM)" htmlFor="cert-pem" required error={errors.certPEM}>
              <Textarea
                id="cert-pem"
                rows={8}
                value={uploadForm.certPEM}
                onChange={(event) => setUploadForm((prev) => ({...prev, certPEM: event.target.value}))}
                placeholder={"-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"}
                className="font-mono text-xs"
              />
            </Field>

            <Field label="Private Key (PEM)" htmlFor="key-pem" required error={errors.keyPEM}>
              <Textarea
                id="key-pem"
                rows={8}
                value={uploadForm.keyPEM}
                onChange={(event) => setUploadForm((prev) => ({...prev, keyPEM: event.target.value}))}
                placeholder={"-----BEGIN EC PRIVATE KEY-----\n...\n-----END EC PRIVATE KEY-----"}
                className="font-mono text-xs"
              />
            </Field>

            <div>
              <Button onClick={handleUpload} loading={submitting}>
                <Lock />
                Upload &amp; Apply Certificate
              </Button>
            </div>
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}

export default CertificateDialog;
