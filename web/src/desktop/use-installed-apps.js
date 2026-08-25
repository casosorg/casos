import {useMemo} from "react";
import * as HelmBackend from "@/backend/HelmBackend";
import * as ImageBackend from "@/backend/ImageBackend";
import * as IngressBackend from "@/backend/IngressBackend";
import * as NodeBackend from "@/backend/NodeBackend";
import * as ServiceBackend from "@/backend/ServiceBackend";
import * as TemplateBackend from "@/backend/TemplateBackend";
import {APP_KIND} from "@/desktop/registry";
import {appResourcesOf, groupAppResources} from "@/lib/appAccess";
import {chartIconUrl} from "@/lib/appCatalog";
import {useResource} from "@/hooks/use-resource";

const POLL_INTERVAL = 30000;

/**
 * The apps this cluster is actually running, as desktop icons.
 *
 * An installed release only becomes an icon once it has an address someone can
 * open — a desktop icon that leads nowhere is worse than no icon. The address
 * comes from the Ingress or Service the release owns, which is the same
 * reading My Apps does.
 */
export function useInstalledApps() {
  const quiet = {initialData: [], toastOnError: false};

  const {data: releases, refresh} = useResource(() => HelmBackend.getHelmReleases("all"), [], {...quiet, pollInterval: POLL_INTERVAL});
  const {data: imageApps} = useResource(() => ImageBackend.getImageApps(""), [], {...quiet, pollInterval: POLL_INTERVAL});
  const {data: services} = useResource(() => ServiceBackend.getServices(), [], {...quiet, pollInterval: POLL_INTERVAL});
  const {data: ingresses} = useResource(() => IngressBackend.getIngresses(), [], {...quiet, pollInterval: POLL_INTERVAL});
  const {data: nodes} = useResource(() => NodeBackend.getNodes(), [], quiet);
  const {data: instances} = useResource(() => TemplateBackend.getTemplateInstances(), [], {...quiet, pollInterval: POLL_INTERVAL});

  const apps = useMemo(() => {
    const grouped = groupAppResources({services, ingresses, pvcs: [], nodes});

    const records = [
      ...(releases ?? []).map((release) => ({
        name: release.name,
        namespace: release.namespace,
        icon: release.icon || chartIconUrl(release.chartName),
      })),
      ...(imageApps ?? []).map((app) => ({
        name: app.name,
        namespace: app.namespace,
        icon: chartIconUrl(String(app.repository ?? "").split("/").pop()),
      })),
    ];

    const clusterApps = records
      .map((record) => {
        const {urls} = appResourcesOf(grouped, record);
        if (urls.length === 0) {
          return null;
        }
        return {
          key: `app-${record.namespace}-${record.name}`,
          name: record.name,
          iconUrl: record.icon || undefined,
          tint: "teal",
          kind: APP_KIND.IFRAME,
          url: urls[0],
          urls,
          installed: true,
        };
      })
      .filter(Boolean);

    // An app from the template market states its own address and icon, so it
    // becomes an icon the moment it is deployed rather than once the cluster
    // has caught up with its Ingress.
    const templateApps = (instances ?? []).flatMap((instance) =>
      (instance.apps ?? [])
        .filter((app) => app.url)
        .map((app) => ({
          key: `template-${instance.namespace}-${instance.name}`,
          name: instance.title || app.name || instance.name,
          iconUrl: app.icon || instance.icon || undefined,
          tint: "rose",
          kind: APP_KIND.IFRAME,
          url: app.url,
          urls: [app.url],
          installed: true,
        }))
    );

    return [...clusterApps, ...templateApps];
  }, [releases, imageApps, services, ingresses, nodes, instances]);

  return {apps, refresh};
}
