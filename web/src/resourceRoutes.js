export function resourcePath(kind, namespace, name) {
  const normalizedKind = String(kind || "").toLowerCase();
  const encodedName = encodeURIComponent(name || "");
  const encodedNamespace = encodeURIComponent(namespace || "");
  switch (normalizedKind) {
  case "node":
    return `/nodes/${encodedName}`;
  case "pod":
    return `/pods/${encodedNamespace}/${encodedName}`;
  case "deployment":
    return `/deployments/${encodedNamespace}/${encodedName}`;
  case "statefulset":
    return `/statefulsets/${encodedNamespace}/${encodedName}`;
  case "daemonset":
    return `/daemonsets/${encodedNamespace}/${encodedName}`;
  case "persistentvolumeclaim":
  case "pvc":
    return `/pvcs/${encodedNamespace}/${encodedName}`;
  case "namespace":
    return `/namespaces/${encodedName}`;
  default:
    return "";
  }
}

export function resourceLabel(resource) {
  if (!resource) {return "-";}
  const name = resource.namespace ? `${resource.namespace}/${resource.name}` : resource.name;
  return `${resource.kind || "-"} / ${name || "-"}`;
}
