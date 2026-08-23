import {Link} from "react-router-dom";
import i18next from "i18next";
import {findLeaf, homePath, navKeyForPath} from "@/nav";
import {useUiMode} from "@/hooks/use-ui-mode";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";

/**
 * Derives the trail from the URL against the shared nav tree, so a page never
 * has to declare its own breadcrumb. A path that names no nav entry renders
 * nothing rather than an invented label.
 */
export function BreadcrumbBar({uri}) {
  const {mode} = useUiMode();
  const segments = (uri || "").split("/").filter(Boolean);
  if (segments.length === 0) {
    return null;
  }

  const navKey = navKeyForPath(uri);
  const leaf = findLeaf(navKey);
  if (!leaf) {
    return null;
  }
  const label = i18next.t(leaf.label);
  // Whatever the URL carries past the nav entry — a chart source, a machine
  // name — is this page's own subject and becomes the last crumb.
  const rest = segments.slice(navKey.split("/").filter(Boolean).length);

  // The home page is what the Home crumb already links to; naming it twice
  // reads as a broken trail rather than a short one.
  if (rest.length === 0 && leaf.path === homePath(mode)) {
    return (
      <Breadcrumb>
        <BreadcrumbList className="text-xs sm:gap-1.5">
          <BreadcrumbItem>
            <BreadcrumbPage>{label}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>
    );
  }

  return (
    <Breadcrumb>
      <BreadcrumbList className="text-xs sm:gap-1.5">
        <BreadcrumbItem>
          <BreadcrumbLink asChild>
            <Link to="/">{i18next.t("general:Home")}</Link>
          </BreadcrumbLink>
        </BreadcrumbItem>
        <BreadcrumbSeparator />
        {rest.length === 0 ? (
          <BreadcrumbItem>
            <BreadcrumbPage>{label}</BreadcrumbPage>
          </BreadcrumbItem>
        ) : (
          <>
            <BreadcrumbItem>
              <BreadcrumbLink asChild>
                <Link to={navKey}>{label}</Link>
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage className="max-w-[240px] truncate">
                {decodeURIComponent(rest[rest.length - 1])}
              </BreadcrumbPage>
            </BreadcrumbItem>
          </>
        )}
      </BreadcrumbList>
    </Breadcrumb>
  );
}
