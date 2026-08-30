import React, {useEffect, useState} from "react";
import i18next from "i18next";
import {RotateCcw} from "lucide-react";
import * as DatabaseBackend from "@/backend/DatabaseBackend";
import {Button} from "@/components/ui/button";
import {Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle} from "@/components/ui/dialog";
import {Input} from "@/components/ui/input";
import {Label} from "@/components/ui/label";
import {MessageAlert} from "@/components/ui/alert";
import {Tabs, TabsContent, TabsList, TabsTrigger} from "@/components/ui/tabs";
import {Loading} from "@/components/shared/loading";
import {SimpleSelect} from "@/components/shared/simple-select";
import {runAction} from "@/hooks/use-resource";

function ParamField({param, value, onChange}) {
  const changed = value !== param.default;

  return (
    <div className="grid gap-1.5">
      <div className="flex items-center justify-between gap-2">
        <Label className="text-sm">{param.label}</Label>
        {changed ? (
          <button
            type="button"
            className="text-muted-foreground hover:text-foreground flex items-center gap-1 text-xs"
            onClick={() => onChange(param.default)}
          >
            <RotateCcw className="size-3" />
            {i18next.t("database:Back to {{value}}", {value: param.default})}
          </button>
        ) : null}
      </div>
      {param.options?.length > 0 ? (
        <SimpleSelect
          value={value}
          onChange={onChange}
          options={param.options.map((option) => ({label: option, value: option}))}
        />
      ) : (
        <Input value={value} onChange={(event) => onChange(event.target.value)} className="h-8 font-mono text-xs" />
      )}
      <p className="text-muted-foreground text-xs">
        <code className="font-mono">{param.key}</code>
        {param.hint ? ` — ${param.hint}` : ""}
      </p>
    </div>
  );
}

function History({entries}) {
  if (entries.length === 0) {
    return <p className="text-muted-foreground py-8 text-center text-sm">{i18next.t("database:Nothing has been changed yet.")}</p>;
  }
  return (
    <div className="grid gap-3">
      {entries.map((entry, index) => (
        <div key={`${entry.at}-${index}`} className="rounded-lg border p-3">
          <p className="text-muted-foreground text-xs">{entry.at}</p>
          <div className="mt-1.5 grid gap-1">
            {entry.changes.map((change) => (
              <p key={change.key} className="font-mono text-xs">
                {change.key}: <span className="text-muted-foreground line-through">{change.from}</span> → {change.to}
              </p>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

/**
 * Engine settings, and the record of who moved them.
 *
 * These reach the engine on its own command line, which it reads once at
 * startup — so saving restarts the database. That is stated rather than hidden,
 * and it is why this is a deliberate action of its own rather than part of
 * editing the database's size.
 */
export function DatabaseParamsDialog({namespace, name, open, onOpenChange, onSaved}) {
  const [data, setData] = useState(null);
  const [values, setValues] = useState({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!open) {
      return;
    }
    setLoading(true);
    setError(null);
    DatabaseBackend.getDatabaseParams(namespace, name)
      .then((res) => {
        if (res.status === "ok") {
          setData(res.data);
          setValues({...res.data.values});
        } else {
          setError(res.msg);
        }
      })
      .catch((requestError) => setError(requestError.message))
      .finally(() => setLoading(false));
  }, [open, namespace, name]);

  function save() {
    setSubmitting(true);
    runAction(DatabaseBackend.configureDatabase({namespace, name, params: values}), {
      successMessage: i18next.t("database:Applying the new settings"),
      onSuccess: () => {
        onOpenChange(false);
        onSaved?.();
      },
    }).finally(() => setSubmitting(false));
  }

  const dirty = data ? Object.keys(values).some((key) => values[key] !== data.values[key]) : false;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{i18next.t("database:Engine settings")}</DialogTitle>
          <DialogDescription>
            {i18next.t("database:The engine reads these when it starts, so saving restarts the database.")}
          </DialogDescription>
        </DialogHeader>

        {error ? <MessageAlert title={error} /> : null}
        {loading ? <Loading /> : null}

        {data ? (
          <Tabs defaultValue="settings">
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="settings">{i18next.t("general:Settings")}</TabsTrigger>
              <TabsTrigger value="history">
                {i18next.t("general:History")}
                {data.history.length > 0 ? ` (${data.history.length})` : ""}
              </TabsTrigger>
            </TabsList>
            <TabsContent value="settings" className="grid gap-4 pt-2">
              {data.params.map((param) => (
                <ParamField
                  key={param.key}
                  param={param}
                  value={values[param.key] ?? param.default}
                  onChange={(next) => setValues((current) => ({...current, [param.key]: next}))}
                />
              ))}
            </TabsContent>
            <TabsContent value="history" className="pt-2">
              <History entries={data.history ?? []} />
            </TabsContent>
          </Tabs>
        ) : null}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {i18next.t("general:Cancel")}
          </Button>
          <Button onClick={save} disabled={!dirty || submitting}>
            {i18next.t("database:Apply and restart")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default DatabaseParamsDialog;
