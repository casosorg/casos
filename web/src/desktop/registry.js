import {
  Activity,
  AppWindow,
  Boxes,
  Container,
  Database,
  FileKey,
  FileText,
  Gauge,
  Globe,
  HardDrive,
  Key,
  Laptop,
  Layers,
  LayoutDashboard,
  Network,
  Repeat,
  Rocket,
  ScrollText,
  Server,
  Settings,
  Shield,
  ShieldCheck,
  Store,
  TerminalSquare,
  Timer,
  UserCog,
  Users,
  Workflow,
} from "lucide-react";

/**
 * The desktop's application catalogue.
 *
 * A desktop app is a window, not a page: the same React screens the sidebar UI
 * serves are mounted inside a window frame, each with a router of its own, so a
 * reader can keep the pod list open while installing a chart next to it. Apps
 * that are not ours — an installed chart's own web UI — are the same object
 * with `kind: "iframe"`, which is how sealos treats every one of its apps.
 */
export const APP_KIND = {
  /** A casos route rendered inside the window. */
  INTERNAL: "internal",
  /** An external address rendered in an iframe inside the window. */
  IFRAME: "iframe",
  /** An external address that opens in a browser tab instead of a window. */
  LINK: "link",
};

/**
 * Icon tints, spelled out rather than composed, because Tailwind only emits the
 * classes it can see in the source.
 */
const TINTS = {
  sky: "bg-gradient-to-br from-sky-400 to-sky-600",
  blue: "bg-gradient-to-br from-blue-400 to-blue-600",
  indigo: "bg-gradient-to-br from-indigo-400 to-indigo-600",
  violet: "bg-gradient-to-br from-violet-400 to-violet-600",
  fuchsia: "bg-gradient-to-br from-fuchsia-400 to-fuchsia-600",
  rose: "bg-gradient-to-br from-rose-400 to-rose-600",
  orange: "bg-gradient-to-br from-orange-400 to-orange-600",
  amber: "bg-gradient-to-br from-amber-400 to-amber-600",
  emerald: "bg-gradient-to-br from-emerald-400 to-emerald-600",
  teal: "bg-gradient-to-br from-teal-400 to-teal-600",
  cyan: "bg-gradient-to-br from-cyan-400 to-cyan-600",
  slate: "bg-gradient-to-br from-slate-400 to-slate-600",
  zinc: "bg-gradient-to-br from-zinc-400 to-zinc-600",
};

export function tintClass(tint) {
  return TINTS[tint] ?? TINTS.zinc;
}

/**
 * Every app the desktop can launch. `path` is the address its window opens at,
 * resolved against the same route table the sidebar UI uses, so an app and its
 * sidebar page can never drift apart.
 */
export const SYSTEM_APPS = [
  {key: "system-dashboard", labelKey: "general:Dashboard", icon: LayoutDashboard, tint: "sky", path: "/dashboard"},
  {key: "system-app-store", labelKey: "general:App Store", icon: Store, tint: "violet", path: "/app-store"},
  {key: "system-my-apps", labelKey: "helm:Installed Apps", icon: Boxes, tint: "amber", path: "/helm-releases"},
  {key: "system-launchpad", labelKey: "general:Deployments", icon: Rocket, tint: "blue", path: "/deployments"},
  {key: "system-terminal", labelKey: "desktop:Terminal", icon: TerminalSquare, tint: "zinc", path: "/terminal"},
  {key: "system-monitor", labelKey: "general:Monitor Center", icon: Gauge, tint: "emerald", path: "/monitor"},
  {key: "system-log-search", labelKey: "general:Log Search", icon: ScrollText, tint: "orange", path: "/log-search"},
  {key: "system-topology", labelKey: "general:Resource Topology", icon: Workflow, tint: "fuchsia", path: "/topology"},
  {key: "system-machines", labelKey: "general:Machines", icon: Laptop, tint: "slate", path: "/machines"},
  {key: "system-nodes", labelKey: "general:Nodes", icon: Server, tint: "indigo", path: "/nodes"},
  {key: "system-pods", labelKey: "general:Pods", icon: Container, tint: "cyan", path: "/pods"},
  {key: "system-statefulsets", labelKey: "general:Stateful Sets", icon: Database, tint: "teal", path: "/statefulsets"},
  {key: "system-daemonsets", labelKey: "general:Daemon Sets", icon: Layers, tint: "blue", path: "/daemonsets"},
  {key: "system-jobs", labelKey: "general:Jobs", icon: Timer, tint: "amber", path: "/jobs"},
  {key: "system-cronjobs", labelKey: "general:Cron Jobs", icon: Repeat, tint: "orange", path: "/cronjobs"},
  {key: "system-namespaces", labelKey: "general:Namespaces", icon: AppWindow, tint: "violet", path: "/namespaces"},
  {key: "system-services", labelKey: "general:Services", icon: Network, tint: "sky", path: "/services"},
  {key: "system-ingresses", labelKey: "general:Ingresses", icon: Globe, tint: "emerald", path: "/ingresses"},
  {key: "system-networkpolicies", labelKey: "general:Network Policies", icon: Shield, tint: "rose", path: "/networkpolicies"},
  {key: "system-configmaps", labelKey: "general:ConfigMaps", icon: FileText, tint: "slate", path: "/configmaps"},
  {key: "system-secrets", labelKey: "general:Secrets", icon: FileKey, tint: "zinc", path: "/secrets"},
  {key: "system-pvcs", labelKey: "general:Persistent Volume Claims", icon: HardDrive, tint: "cyan", path: "/pvcs"},
  {key: "system-storageclasses", labelKey: "general:Storage Classes", icon: HardDrive, tint: "teal", path: "/storageclasses"},
  {key: "system-resourcequotas", labelKey: "general:Resource Quotas", icon: Gauge, tint: "indigo", path: "/resourcequotas"},
  {key: "system-hpas", labelKey: "general:Horizontal Pod Autoscaler", icon: Activity, tint: "emerald", path: "/hpas"},
  {key: "system-serviceaccounts", labelKey: "general:ServiceAccounts", icon: Users, tint: "blue", path: "/serviceaccounts"},
  {key: "system-rolebindings", labelKey: "general:Role Bindings", icon: UserCog, tint: "violet", path: "/rolebindings"},
  {key: "system-clusterrolebindings", labelKey: "general:ClusterRoleBindings", icon: Key, tint: "fuchsia", path: "/clusterrolebindings"},
  {key: "system-admission-policy", labelKey: "general:Admission Policy", icon: ShieldCheck, tint: "rose", path: "/admission-policy"},
  {key: "system-authorization-policy", labelKey: "general:Authorization Policy", icon: Shield, tint: "orange", path: "/authorization-policy"},
  {key: "system-trivy-scans", labelKey: "general:Image Scan", icon: ShieldCheck, tint: "amber", path: "/trivy-scans"},
  {key: "system-sites", labelKey: "general:Sites", icon: Settings, tint: "slate", path: "/sites/site-built-in"},
].map((app) => ({...app, kind: APP_KIND.INTERNAL, system: true}));

/** Apps the desktop shows before anyone rearranges it. The rest live in the folder. */
export const DEFAULT_DESKTOP_KEYS = [
  "system-app-store",
  "system-my-apps",
  "system-launchpad",
  "system-terminal",
  "system-dashboard",
  "system-monitor",
  "system-log-search",
  "system-machines",
  "system-nodes",
  "system-pods",
];

/** The five icons the dock keeps pinned, in order, alongside anything running. */
export const DOCK_PINNED_KEYS = [
  "system-app-store",
  "system-my-apps",
  "system-launchpad",
  "system-terminal",
  "system-monitor",
];
