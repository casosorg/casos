import React, {useEffect, useMemo, useState} from "react";
import i18next from "i18next";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import * as PodBackend from "@/backend/PodBackend";
import {EmptyState} from "@/components/shared/empty-state";
import {PodShell} from "@/components/shared/pod-shell";
import {SimpleSelect} from "@/components/shared/simple-select";
import {TerminalSquare} from "lucide-react";
import {useResource} from "@/hooks/use-resource";

/**
 * The Terminal app: a shell into any container in the cluster, reached by
 * picking one rather than by finding its pod on a list page first.
 */
function TerminalPage() {
  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: [], toastOnError: false});
  const [namespace, setNamespace] = useState("");
  const [podName, setPodName] = useState("");
  const [container, setContainer] = useState("");

  const {data: pods, loading} = useResource(() => PodBackend.getPods(namespace), [namespace], {initialData: [], toastOnError: false});

  useEffect(() => {
    if (!namespace && namespaces.length > 0) {
      const preferred = namespaces.find((item) => item.name === "default") ?? namespaces[0];
      setNamespace(preferred.name);
    }
  }, [namespaces, namespace]);

  const running = useMemo(() => pods.filter((pod) => (pod.status ?? pod.phase) === "Running"), [pods]);

  useEffect(() => {
    const pod = running.find((item) => item.name === podName) ?? running[0];
    setPodName(pod?.name ?? "");
    setContainer(pod?.containers?.[0] ?? "");
    // Switching namespaces has to land on a pod that exists in the new one.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [running]);

  const pod = running.find((item) => item.name === podName) ?? null;
  const containers = pod?.containers ?? [];

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex flex-wrap items-center gap-2 border-b p-3">
        <SimpleSelect
          value={namespace}
          onChange={setNamespace}
          options={namespaces.map((item) => ({label: item.name, value: item.name}))}
          size="sm"
          className="w-52"
          placeholder={i18next.t("general:Namespace")}
        />
        <SimpleSelect
          value={podName}
          onChange={(next) => {
            setPodName(next);
            setContainer(running.find((item) => item.name === next)?.containers?.[0] ?? "");
          }}
          options={running.map((item) => ({label: item.name, value: item.name}))}
          size="sm"
          className="w-72"
          placeholder={i18next.t("general:Pods")}
        />
        {containers.length > 1 && (
          <SimpleSelect
            value={container}
            onChange={setContainer}
            options={containers.map((name) => ({label: name, value: name}))}
            size="sm"
            className="w-44"
            placeholder={i18next.t("desktop:Container")}
          />
        )}
      </div>

      <div className="min-h-0 flex-1 bg-neutral-950 p-3">
        {pod ? (
          <PodShell
            key={`${pod.namespace}/${pod.name}/${container}`}
            namespace={pod.namespace}
            name={pod.name}
            container={container}
            className="h-full"
          />
        ) : (
          <div className="flex h-full items-center justify-center">
            <EmptyState
              icon={TerminalSquare}
              title={loading ? i18next.t("desktop:Loading pods") : i18next.t("desktop:No running pod to attach to")}
              description={i18next.t("desktop:Pick a namespace that has a running pod.")}
            />
          </div>
        )}
      </div>
    </div>
  );
}

export default TerminalPage;
