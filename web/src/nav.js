import {
  Activity,
  AppWindow,
  Boxes,
  ClipboardList,
  Cog,
  Gauge,
  House,
  Laptop,
  LayoutDashboard,
  Lock,
  Network,
  Rocket,
  Server,
  Store,
} from "lucide-react";

/**
 * One description of the navigation, read by both the sidebar and the
 * breadcrumb. Keeping them on the same tree is what stops the two from
 * disagreeing about which section a page belongs to — the old UI maintained
 * that mapping twice and they drifted.
 *
 * `label` is an i18next key, resolved at render time so a language switch does
 * not need the tree rebuilt.
 */
export const navGroups = [
  {key: "/dashboard", label: "general:Dashboard", icon: LayoutDashboard, path: "/dashboard"},
  {key: "/app-store", label: "general:App Store", icon: Store, path: "/app-store"},
  {key: "/launchpad", label: "launchpad:App Launchpad", icon: Rocket, path: "/launchpad"},
  {key: "/helm-releases", label: "helm:Installed Apps", icon: Boxes, path: "/helm-releases"},
  {
    key: "/workloads",
    label: "general:Workloads",
    icon: AppWindow,
    children: [
      {key: "/pods", label: "general:Pods", path: "/pods"},
      {key: "/deployments", label: "general:Deployments", path: "/deployments"},
      {key: "/statefulsets", label: "general:Stateful Sets", path: "/statefulsets"},
      {key: "/daemonsets", label: "general:Daemon Sets", path: "/daemonsets"},
      {key: "/jobs", label: "general:Jobs", path: "/jobs"},
      {key: "/cronjobs", label: "general:Cron Jobs", path: "/cronjobs"},
    ],
  },
  {
    key: "/cluster",
    label: "general:Cluster",
    icon: Server,
    children: [
      {key: "/nodes", label: "general:Nodes", path: "/nodes"},
      {key: "/namespaces", label: "general:Namespaces", path: "/namespaces"},
      {key: "/serviceaccounts", label: "general:ServiceAccounts", path: "/serviceaccounts"},
    ],
  },
  {
    key: "/configuration",
    label: "general:Configuration",
    icon: Cog,
    children: [
      {key: "/configmaps", label: "general:ConfigMaps", path: "/configmaps"},
      {key: "/secrets", label: "general:Secrets", path: "/secrets"},
      {key: "/pvcs", label: "general:Persistent Volume Claims", path: "/pvcs"},
      {key: "/storageclasses", label: "general:Storage Classes", path: "/storageclasses"},
      {key: "/resourcequotas", label: "general:Resource Quotas", path: "/resourcequotas"},
      {key: "/hpas", label: "general:Horizontal Pod Autoscaler", path: "/hpas"},
    ],
  },
  {
    key: "/networking",
    label: "general:Networking",
    icon: Network,
    children: [
      {key: "/services", label: "general:Services", path: "/services"},
      {key: "/ingresses", label: "general:Ingresses", path: "/ingresses"},
      {key: "/networkpolicies", label: "general:Network Policies", path: "/networkpolicies"},
    ],
  },
  {
    key: "/accesscontrol",
    label: "general:Access Control",
    icon: Lock,
    children: [
      {key: "/rolebindings", label: "general:Role Bindings", path: "/rolebindings"},
      {key: "/clusterrolebindings", label: "general:ClusterRoleBindings", path: "/clusterrolebindings"},
      {key: "/admission-policy", label: "general:Admission Policy", path: "/admission-policy"},
      {key: "/authorization-policy", label: "general:Authorization Policy", path: "/authorization-policy"},
      {key: "/trivy-scans", label: "general:Image Scan", path: "/trivy-scans"},
    ],
  },
  {
    key: "/observability",
    label: "general:Observability",
    icon: Gauge,
    children: [
      {key: "/monitor", label: "general:Monitor Center", path: "/monitor"},
      {key: "/log-search", label: "general:Log Search", path: "/log-search"},
      {key: "/topology", label: "general:Resource Topology", path: "/topology"},
    ],
  },
  {
    key: "/infrastructure",
    label: "general:Infrastructure",
    icon: Server,
    children: [{key: "/machines", label: "general:Machines", path: "/machines"}],
  },
  {
    key: "/admin",
    label: "general:Admin",
    icon: ClipboardList,
    children: [{key: "/sites", label: "general:Sites", path: "/sites/site-built-in"}],
  },
];

/** Simple mode's pages live under this prefix, so no URL ever serves both modes. */
export const SIMPLE_PREFIX = "/simple";

/** The desktop is a mode of its own: one address, every page inside a window. */
export const DESKTOP_PREFIX = "/desktop";

export function isDesktopPath(pathname) {
  return pathname === DESKTOP_PREFIX || (pathname ?? "").startsWith(`${DESKTOP_PREFIX}/`);
}

/**
 * Simple mode's navigation: five flat entries named after what a reader wants to
 * do, not after the Kubernetes object behind it.
 *
 * There is deliberately no entry for addresses or for storage. Neither is a
 * place someone goes — they are things one app has, so they are shown on that
 * app's card under My Apps. Devices and Health put two of the advanced list
 * pages behind tabs, so no functionality is lost, only reached differently.
 */
export const simpleNavGroups = [
  {key: "/simple", label: "simple:Home", icon: House, path: "/simple"},
  {key: "/simple/app-store", label: "simple:App Store", icon: Store, path: "/simple/app-store"},
  {key: "/simple/apps", label: "simple:My Apps", icon: Boxes, path: "/simple/apps"},
  {key: "/simple/devices", label: "simple:Devices", icon: Laptop, path: "/simple/devices"},
  {key: "/simple/health", label: "simple:Health", icon: Activity, path: "/simple/health"},
];

export function getNavGroups(mode) {
  return mode === "advanced" ? navGroups : simpleNavGroups;
}

/** All leaf entries, flattened, for lookups by nav key. */
export const navLeaves = navGroups.flatMap((group) => (group.children ? group.children : [group]));

export function isSimplePath(pathname) {
  return pathname === SIMPLE_PREFIX || (pathname ?? "").startsWith(`${SIMPLE_PREFIX}/`);
}

/**
 * The nav entry a URL belongs to: its first segment, or its first two under
 * /simple. Everything past that — a chart source, a machine name — is the
 * page's own subject, not another entry.
 */
export function navKeyForPath(pathname) {
  const segments = (pathname || "").split("/").filter(Boolean);
  if (segments.length === 0) {
    return "/dashboard";
  }
  if (`/${segments[0]}` === SIMPLE_PREFIX) {
    return segments.length > 1 ? `${SIMPLE_PREFIX}/${segments[1]}` : SIMPLE_PREFIX;
  }
  return `/${segments[0]}`;
}

// Which advanced page each simple page stands in for, so switching mode lands on
// the same subject rather than dropping the reader on the home page. Read in
// both directions: the first pair naming a side wins, which is what lets two
// advanced pages fold into one simple page without making the reverse ambiguous.
const MODE_COUNTERPARTS = [
  ["/dashboard", "/simple"],
  ["/app-store", "/simple/app-store"],
  ["/helm-releases", "/simple/apps"],
  ["/machines", "/simple/devices"],
  ["/nodes", "/simple/devices"],
  ["/monitor", "/simple/health"],
  ["/log-search", "/simple/health"],
];

export function homePath(mode) {
  if (mode === "desktop") {
    return DESKTOP_PREFIX;
  }
  return mode === "advanced" ? "/dashboard" : SIMPLE_PREFIX;
}

/** The path a page shared by both modes should link to from the given mode. */
export function pathForMode(advancedPath, mode) {
  if (mode === "advanced") {
    return advancedPath;
  }
  const pair = MODE_COUNTERPARTS.find(([advanced]) => advanced === navKeyForPath(advancedPath));
  return pair ? pair[1] : advancedPath;
}

/** Where a URL's reader should land after switching to the other mode. */
export function counterpartPath(pathname, targetMode) {
  // The desktop has one address and hosts every page inside it, so there is no
  // per-page counterpart to look up in either direction.
  if (targetMode === "desktop" || isDesktopPath(pathname)) {
    return homePath(targetMode);
  }
  const key = navKeyForPath(pathname);
  const pair = targetMode === "advanced"
    ? MODE_COUNTERPARTS.find(([, simple]) => simple === key)
    : MODE_COUNTERPARTS.find(([advanced]) => advanced === key);
  if (!pair) {
    return homePath(targetMode);
  }
  return targetMode === "advanced" ? pair[0] : pair[1];
}

// Both trees are searched: a nav key belongs to exactly one of them now that
// simple mode's pages have URLs of their own, and the breadcrumb has to name
// /pods in either mode.
export function findLeaf(navKey) {
  return navLeaves.find((leaf) => leaf.key === navKey) ?? simpleNavGroups.find((leaf) => leaf.key === navKey);
}

export function findGroupOf(navKey) {
  return navGroups.find((group) => group.children?.some((child) => child.key === navKey));
}
