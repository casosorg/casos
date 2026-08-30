/**
 * The apps simple mode offers. Every entry is a chart the backend has an
 * install adapter for (see store/adapters.go), which is what makes them work
 * without anyone editing values.yaml — that, rather than popularity, is the
 * bar for being listed here. Anything else is still installable from the full
 * App Store in advanced mode.
 *
 * `description` doubles as the i18next key in the `simple` namespace.
 */

const BITNAMI = "https://charts.bitnami.com/bitnami";

// ArtifactHub re-hosts the logo every chart declares in its Chart.yaml, at a
// URL that stays the same across chart versions. An installed release carries
// its own icon back from the cluster, so these are only what the App Store
// shows for an app nobody has installed yet — and the fallback for a release
// whose chart declares no icon at all.
function chartIcon(imageId) {
  return `https://artifacthub.io/image/${imageId}`;
}

export const APP_CATEGORIES = [
  {key: "all", label: "simple:Recommended"},
  {key: "files", label: "simple:Files and media"},
  {key: "website", label: "simple:Websites"},
  {key: "database", label: "database:Databases"},
  {key: "devtools", label: "simple:Developer tools"},
  {key: "monitoring", label: "simple:Monitoring"},
  {key: "security", label: "simple:Security and login"},
  {key: "storage", label: "general:Storage"},
];

export const APP_CATALOG = [
  {
    chartName: "nextcloud",
    icon: chartIcon("3fb48919-25f8-475a-aba0-8ff63e3ada3b"),
    name: "Nextcloud",
    repoURL: "https://nextcloud.github.io/helm/",
    category: "files",
    description: "simple:Your own private cloud drive for files, photos and calendars.",
  },
  {
    chartName: "ingress-nginx",
    icon: chartIcon("282abebe-66ad-46f7-991d-abdb72ca01b7"),
    name: "Nginx Ingress",
    repoURL: "https://kubernetes.github.io/ingress-nginx",
    category: "website",
    description: "simple:Lets you put your apps on a domain name instead of a port number.",
  },
  {
    chartName: "traefik",
    icon: chartIcon("f019cbfd-b8d4-43ef-b115-791bba16bf1f"),
    name: "Traefik",
    repoURL: "https://traefik.github.io/charts",
    category: "website",
    description: "simple:An alternative front door that routes web traffic to your apps.",
  },
  {
    chartName: "cert-manager",
    icon: chartIcon("6d75ecff-a328-495b-a7fc-12cd84499974"),
    name: "Cert Manager",
    repoURL: "https://charts.jetstack.io",
    category: "website",
    description: "simple:Gets free HTTPS certificates for your domains and renews them.",
  },
  {
    chartName: "postgresql",
    icon: chartIcon("371d5e57-34c4-46ef-909e-658c1502ced8"),
    name: "PostgreSQL",
    repoURL: BITNAMI,
    category: "database",
    description: "simple:A reliable database other apps can store their data in.",
  },
  {
    chartName: "mysql",
    icon: chartIcon("d45c30b0-a6ce-493b-9f00-0f117e2e7974"),
    name: "MySQL",
    repoURL: BITNAMI,
    category: "database",
    description: "simple:The database most websites and blogs expect.",
  },
  {
    chartName: "mongodb",
    icon: chartIcon("3c9f2531-b14e-49d7-8a2b-12ab2dc7d9ae"),
    name: "MongoDB",
    repoURL: BITNAMI,
    category: "database",
    description: "simple:A database that stores documents instead of tables.",
  },
  {
    chartName: "redis",
    icon: chartIcon("7a9e0976-a969-4542-aecf-21d9b2e4fadb"),
    name: "Redis",
    repoURL: BITNAMI,
    category: "database",
    description: "simple:Keeps frequently used data in memory so apps feel faster.",
  },
  {
    chartName: "elasticsearch",
    icon: chartIcon("a8078b43-787e-4dd1-96e0-5611bacd362b"),
    name: "Elasticsearch",
    repoURL: BITNAMI,
    category: "database",
    description: "simple:Full-text search over large amounts of data.",
  },
  {
    chartName: "kafka",
    icon: chartIcon("da3e7fa0-2d82-43e9-8cad-87bac262e9dc"),
    name: "Kafka",
    repoURL: BITNAMI,
    category: "devtools",
    description: "simple:Passes messages between apps without losing them.",
  },
  {
    chartName: "rabbitmq",
    icon: chartIcon("c428241f-1cae-4b27-aba7-7c3ac67b9a88"),
    name: "RabbitMQ",
    repoURL: BITNAMI,
    category: "devtools",
    description: "simple:A queue that lets apps hand work to each other.",
  },
  {
    chartName: "gitlab",
    icon: chartIcon("b973c749-af26-4492-9a15-ad59b977ecbe"),
    name: "GitLab",
    repoURL: "https://charts.gitlab.io",
    category: "devtools",
    description: "simple:Host your own code, issues and build pipelines.",
  },
  {
    chartName: "jenkins",
    icon: chartIcon("6aedc434-b6b1-41e1-8610-a6ecb664bd14"),
    name: "Jenkins",
    repoURL: "https://charts.jenkins.io",
    category: "devtools",
    description: "simple:Runs your builds and tests automatically.",
  },
  {
    chartName: "argo-cd",
    icon: chartIcon("e16221f4-a3b1-49f6-9b65-cb4dbf30ed83"),
    name: "Argo CD",
    repoURL: "https://argoproj.github.io/argo-helm",
    category: "devtools",
    description: "simple:Keeps what is running in step with what is in your Git repository.",
  },
  {
    chartName: "harbor",
    icon: chartIcon("93376a3e-0c15-4dfd-b747-5f11576321fb"),
    name: "Harbor",
    repoURL: "https://helm.goharbor.io",
    category: "devtools",
    description: "simple:Your own registry for container images.",
  },
  {
    chartName: "n8n",
    icon: chartIcon("d65108f6-ec3b-4100-b003-635e5fc0b880"),
    name: "n8n",
    repoURL: "https://community-charts.github.io/helm-charts",
    category: "devtools",
    description: "simple:Connect apps together and automate tasks by drawing a flow.",
  },
  {
    chartName: "grafana",
    icon: chartIcon("029d6bb0-215f-414e-be87-c71265cd93a1"),
    name: "Grafana",
    repoURL: "https://grafana.github.io/helm-charts",
    category: "monitoring",
    description: "simple:Turns numbers about your cluster into dashboards you can read.",
  },
  {
    chartName: "kube-prometheus-stack",
    icon: chartIcon("0503add5-3fce-4b63-bbf3-b9f649512a86"),
    name: "Prometheus Stack",
    repoURL: "https://prometheus-community.github.io/helm-charts",
    category: "monitoring",
    description: "simple:Collects usage numbers and alerts you when something looks wrong.",
  },
  {
    chartName: "loki",
    icon: chartIcon("6e0c6a8b-99fe-4ed5-a2ff-7799ba373223"),
    name: "Loki",
    repoURL: "https://grafana.github.io/helm-charts",
    category: "monitoring",
    description: "simple:Keeps the messages your apps write so you can search them later.",
  },
  {
    chartName: "metrics-server",
    icon: chartIcon("92b1716e-c455-4113-9a8e-1141b2961479"),
    name: "Metrics Server",
    repoURL: "https://kubernetes-sigs.github.io/metrics-server/",
    category: "monitoring",
    description: "simple:Required before CasOS can show how much CPU and memory apps use.",
  },
  {
    chartName: "kubernetes-dashboard",
    icon: chartIcon("c711f9f9-28b3-4ee8-98a2-30e00abf9f02"),
    name: "Kubernetes Dashboard",
    repoURL: "https://kubernetes.github.io/dashboard/",
    category: "monitoring",
    description: "simple:The official Kubernetes web console, if you prefer it.",
  },
  {
    chartName: "superset",
    icon: chartIcon("9b9ea293-8f1b-4011-a19a-eff6c80a5135"),
    name: "Superset",
    repoURL: "https://apache.github.io/superset",
    category: "monitoring",
    description: "simple:Build charts and reports from data in your databases.",
  },
  {
    chartName: "keycloak",
    icon: chartIcon("84e8d984-f5ae-4573-912f-bb7ca9be0603"),
    name: "Keycloak",
    repoURL: BITNAMI,
    category: "security",
    description: "simple:One login for all your apps, with users and groups you manage.",
  },
  {
    chartName: "vault",
    icon: chartIcon("c8d6d027-b302-49f5-b8aa-964910ed04eb"),
    name: "Vault",
    repoURL: "https://helm.releases.hashicorp.com",
    category: "security",
    description: "simple:Stores passwords and keys so they are not written in plain text.",
  },
  {
    chartName: "longhorn",
    icon: chartIcon("fbd5b075-5f1f-438c-aa78-356c14e158c5"),
    name: "Longhorn",
    repoURL: "https://charts.longhorn.io",
    category: "storage",
    description: "simple:Gives your apps disks that survive a computer going down.",
  },
];

// Charts CasOS does not offer in simple mode, but which someone is likely to
// have installed from the full App Store or found already running in the
// cluster. Their releases would otherwise fall back to a monogram.
const EXTRA_CHART_ICONS = {
  prometheus: chartIcon("0503add5-3fce-4b63-bbf3-b9f649512a86"),
  portainer: chartIcon("7e2d4dbf-544c-4aa7-9f21-5fc7fbcdd8fb"),
  "external-dns": chartIcon("80e6b87a-544d-4794-aa15-02c73884f9cc"),
  "external-secrets": chartIcon("f23fb631-4a3c-46c0-bda1-7a048cabf10d"),
  minio: chartIcon("afd67ebf-7003-41b4-87c0-a74e57b2da22"),
  gitea: chartIcon("44a8fad2-2606-48e0-97c3-453951586c7e"),
  wordpress: chartIcon("2b02241b-36d9-487e-b747-3f37bd4cb9a5"),
  mariadb: chartIcon("252b526a-b40b-4140-82b3-0db4398c43cf"),
  sonarqube: chartIcon("132ce52d-b560-46c0-8163-ed24eef8ec56"),
};

const CHART_ICONS = APP_CATALOG.reduce(
  (icons, app) => (app.icon ? {...icons, [app.chartName]: app.icon} : icons),
  {...EXTRA_CHART_ICONS}
);

export function chartIconUrl(chartName) {
  return CHART_ICONS[chartName] ?? null;
}

// A stable colour per app so the monogram that stands in for a missing logo
// does not change between visits.
const TILE_COLORS = [
  "#2563eb", "#0891b2", "#7c3aed", "#db2777", "#ea580c",
  "#16a34a", "#4f46e5", "#0d9488", "#c026d3", "#dc2626",
];

export function appTileColor(chartName) {
  let hash = 0;
  for (let index = 0; index < (chartName ?? "").length; index += 1) {
    hash = (hash * 31 + chartName.charCodeAt(index)) % 1000003;
  }
  return TILE_COLORS[hash % TILE_COLORS.length];
}

// The darker end of the monogram tile's gradient: the same hue with every
// channel pulled towards black, so one palette entry yields both stops.
export function appTileGradient(chartName) {
  const from = appTileColor(chartName);
  const to = [1, 3, 5]
    .map((offset) => Math.round(parseInt(from.slice(offset, offset + 2), 16) * 0.68))
    .map((channel) => channel.toString(16).padStart(2, "0"))
    .join("");
  return `linear-gradient(140deg, ${from} 0%, #${to} 100%)`;
}
