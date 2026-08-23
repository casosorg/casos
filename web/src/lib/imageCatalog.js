/**
 * What the Docker Hub channel shows before anyone types.
 *
 * Docker Hub ranks a bare search by pull count across every image on the
 * registry — base images, language runtimes, CI artefacts — so an empty query
 * has no useful answer. These are search seeds, not install specs: everything
 * an install actually needs still comes from the image's own config, read at
 * install time.
 *
 * `description` doubles as the i18next key in the `image` namespace.
 */
export const IMAGE_STARTER_APPS = [
  {
    image: "pkpofficial/ojs",
    name: "OJS",
    description: "image:Peer review and publishing for academic journals.",
  },
  {
    image: "wordpress",
    name: "WordPress",
    description: "image:The blog and website platform much of the web runs on.",
  },
  {
    image: "nextcloud",
    name: "Nextcloud",
    description: "image:Your own private cloud drive for files, photos and calendars.",
  },
  {
    image: "gitea/gitea",
    name: "Gitea",
    description: "image:A lightweight Git service you host yourself.",
  },
  {
    image: "n8nio/n8n",
    name: "n8n",
    description: "image:Wires your apps together into automated workflows.",
  },
  {
    image: "grafana/grafana",
    name: "Grafana",
    description: "image:Dashboards and graphs for your metrics.",
  },
  {
    image: "louislam/uptime-kuma",
    name: "Uptime Kuma",
    description: "image:Watches your sites and tells you when one goes down.",
  },
  {
    image: "vaultwarden/server",
    name: "Vaultwarden",
    description: "image:A self-hosted password manager that speaks Bitwarden.",
  },
  {
    image: "jellyfin/jellyfin",
    name: "Jellyfin",
    description: "image:Streams your own films, series and music.",
  },
  {
    image: "mariadb",
    name: "MariaDB",
    description: "image:The database most websites and blogs expect.",
  },
  {
    image: "postgres",
    name: "PostgreSQL",
    description: "image:A reliable database other apps can store their data in.",
  },
  {
    image: "redis",
    name: "Redis",
    description: "image:An in-memory store apps use for caching and queues.",
  },
];

/**
 * The companion database the installer offers when an image's env says it wants
 * one. Keyed by the engine the backend infers from that env.
 */
export const DATABASE_ENGINES = {
  mysql: {
    label: "MariaDB",
    image: "mariadb:11",
    port: 3306,
    mountPath: "/var/lib/mysql",
    env: ({name, user, password, rootPassword}) => [
      {key: "MARIADB_DATABASE", value: name},
      {key: "MARIADB_USER", value: user},
      {key: "MARIADB_PASSWORD", value: password},
      {key: "MARIADB_ROOT_PASSWORD", value: rootPassword},
    ],
  },
  postgres: {
    label: "PostgreSQL",
    image: "postgres:17-alpine",
    port: 5432,
    mountPath: "/var/lib/postgresql/data",
    env: ({name, user, password}) => [
      {key: "POSTGRES_DB", value: name},
      {key: "POSTGRES_USER", value: user},
      {key: "POSTGRES_PASSWORD", value: password},
      // initdb refuses to run in a non-empty directory and a freshly bound PVC
      // arrives carrying lost+found, so the data lives one level down.
      {key: "PGDATA", value: "/var/lib/postgresql/data/pgdata"},
    ],
  },
};
