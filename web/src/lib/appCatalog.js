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

export const APP_CATEGORIES = [
  {key: "all", label: "simple:Recommended"},
  {key: "files", label: "simple:Files and media"},
  {key: "website", label: "simple:Websites"},
  {key: "database", label: "simple:Databases"},
  {key: "devtools", label: "simple:Developer tools"},
  {key: "monitoring", label: "simple:Monitoring"},
  {key: "security", label: "simple:Security and login"},
  {key: "storage", label: "simple:Storage"},
];

export const APP_CATALOG = [
  {
    chartName: "nextcloud",
    name: "Nextcloud",
    repoURL: "https://nextcloud.github.io/helm/",
    category: "files",
    description: "simple:Your own private cloud drive for files, photos and calendars.",
  },
  {
    chartName: "ingress-nginx",
    name: "Nginx Ingress",
    repoURL: "https://kubernetes.github.io/ingress-nginx",
    category: "website",
    description: "simple:Lets you put your apps on a domain name instead of a port number.",
  },
  {
    chartName: "traefik",
    name: "Traefik",
    repoURL: "https://traefik.github.io/charts",
    category: "website",
    description: "simple:An alternative front door that routes web traffic to your apps.",
  },
  {
    chartName: "cert-manager",
    name: "Cert Manager",
    repoURL: "https://charts.jetstack.io",
    category: "website",
    description: "simple:Gets free HTTPS certificates for your domains and renews them.",
  },
  {
    chartName: "postgresql",
    name: "PostgreSQL",
    repoURL: BITNAMI,
    category: "database",
    description: "simple:A reliable database other apps can store their data in.",
  },
  {
    chartName: "mysql",
    name: "MySQL",
    repoURL: BITNAMI,
    category: "database",
    description: "simple:The database most websites and blogs expect.",
  },
  {
    chartName: "mongodb",
    name: "MongoDB",
    repoURL: BITNAMI,
    category: "database",
    description: "simple:A database that stores documents instead of tables.",
  },
  {
    chartName: "redis",
    name: "Redis",
    repoURL: BITNAMI,
    category: "database",
    description: "simple:Keeps frequently used data in memory so apps feel faster.",
  },
  {
    chartName: "elasticsearch",
    name: "Elasticsearch",
    repoURL: BITNAMI,
    category: "database",
    description: "simple:Full-text search over large amounts of data.",
  },
  {
    chartName: "kafka",
    name: "Kafka",
    repoURL: BITNAMI,
    category: "devtools",
    description: "simple:Passes messages between apps without losing them.",
  },
  {
    chartName: "rabbitmq",
    name: "RabbitMQ",
    repoURL: BITNAMI,
    category: "devtools",
    description: "simple:A queue that lets apps hand work to each other.",
  },
  {
    chartName: "gitlab",
    name: "GitLab",
    repoURL: "https://charts.gitlab.io",
    category: "devtools",
    description: "simple:Host your own code, issues and build pipelines.",
  },
  {
    chartName: "jenkins",
    name: "Jenkins",
    repoURL: "https://charts.jenkins.io",
    category: "devtools",
    description: "simple:Runs your builds and tests automatically.",
  },
  {
    chartName: "argo-cd",
    name: "Argo CD",
    repoURL: "https://argoproj.github.io/argo-helm",
    category: "devtools",
    description: "simple:Keeps what is running in step with what is in your Git repository.",
  },
  {
    chartName: "harbor",
    name: "Harbor",
    repoURL: "https://helm.goharbor.io",
    category: "devtools",
    description: "simple:Your own registry for container images.",
  },
  {
    chartName: "n8n",
    name: "n8n",
    repoURL: "https://community-charts.github.io/helm-charts",
    category: "devtools",
    description: "simple:Connect apps together and automate tasks by drawing a flow.",
  },
  {
    chartName: "grafana",
    name: "Grafana",
    repoURL: "https://grafana.github.io/helm-charts",
    category: "monitoring",
    description: "simple:Turns numbers about your cluster into dashboards you can read.",
  },
  {
    chartName: "kube-prometheus-stack",
    name: "Prometheus Stack",
    repoURL: "https://prometheus-community.github.io/helm-charts",
    category: "monitoring",
    description: "simple:Collects usage numbers and alerts you when something looks wrong.",
  },
  {
    chartName: "loki",
    name: "Loki",
    repoURL: "https://grafana.github.io/helm-charts",
    category: "monitoring",
    description: "simple:Keeps the messages your apps write so you can search them later.",
  },
  {
    chartName: "metrics-server",
    name: "Metrics Server",
    repoURL: "https://kubernetes-sigs.github.io/metrics-server/",
    category: "monitoring",
    description: "simple:Required before CasOS can show how much CPU and memory apps use.",
  },
  {
    chartName: "kubernetes-dashboard",
    name: "Kubernetes Dashboard",
    repoURL: "https://kubernetes.github.io/dashboard/",
    category: "monitoring",
    description: "simple:The official Kubernetes web console, if you prefer it.",
  },
  {
    chartName: "superset",
    name: "Superset",
    repoURL: "https://apache.github.io/superset",
    category: "monitoring",
    description: "simple:Build charts and reports from data in your databases.",
  },
  {
    chartName: "keycloak",
    name: "Keycloak",
    repoURL: BITNAMI,
    category: "security",
    description: "simple:One login for all your apps, with users and groups you manage.",
  },
  {
    chartName: "vault",
    name: "Vault",
    repoURL: "https://helm.releases.hashicorp.com",
    category: "security",
    description: "simple:Stores passwords and keys so they are not written in plain text.",
  },
  {
    chartName: "longhorn",
    name: "Longhorn",
    repoURL: "https://charts.longhorn.io",
    category: "storage",
    description: "simple:Gives your apps disks that survive a computer going down.",
  },
];

// A stable colour per app so the tile does not change between visits. Charts do
// not ship a logo the browser can reach without an extra request per card, so
// the first letter on a coloured tile stands in for one.
const TILE_COLORS = [
  "#2563eb", "#0891b2", "#7c3aed", "#db2777", "#ea580c",
  "#16a34a", "#4f46e5", "#0d9488", "#c026d3", "#dc2626",
];

export function appTileColor(chartName) {
  let hash = 0;
  for (let index = 0; index < chartName.length; index += 1) {
    hash = (hash * 31 + chartName.charCodeAt(index)) % 1000003;
  }
  return TILE_COLORS[hash % TILE_COLORS.length];
}
