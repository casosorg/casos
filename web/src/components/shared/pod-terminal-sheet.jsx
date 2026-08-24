import React, {useEffect, useState} from "react";
import {PodShell} from "@/components/shared/pod-shell";
import {ResourceSheet} from "@/components/shared/resource-sheet";
import {SimpleSelect} from "@/components/shared/simple-select";

/** An interactive shell inside one container of a pod. */
export function PodTerminalSheet({pod, open, onClose}) {
  const [container, setContainer] = useState("");

  useEffect(() => {
    setContainer(pod?.containers?.[0] ?? "");
  }, [pod]);

  const containerOptions = (pod?.containers ?? []).map((name) => ({label: name, value: name}));

  return (
    <ResourceSheet
      open={open}
      onOpenChange={(next) => (next ? null : onClose())}
      title={pod ? `Terminal — ${pod.namespace} / ${pod.name}` : "Terminal"}
      size="xl"
      bodyClassName="bg-neutral-950 p-3"
      toolbar={
        containerOptions.length > 1 ? (
          <SimpleSelect
            value={container}
            onChange={setContainer}
            options={containerOptions}
            size="sm"
            className="w-44"
            placeholder="Container"
          />
        ) : null
      }
    >
      {open && pod ? (
        <PodShell
          key={`${pod.namespace}/${pod.name}/${container}`}
          namespace={pod.namespace}
          name={pod.name}
          container={container}
          openDelay={250}
        />
      ) : null}
    </ResourceSheet>
  );
}

export default PodTerminalSheet;
