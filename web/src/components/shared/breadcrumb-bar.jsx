import {Link} from "react-router-dom";
import i18next from "i18next";
import {findLeaf} from "@/nav";
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
 * has to declare its own breadcrumb. A path whose first segment is not in the
 * tree renders nothing rather than an invented label.
 */
export function BreadcrumbBar({uri}) {
  const {mode} = useUiMode();
  const segments = (uri || "").split("/").filter(Boolean);
  if (segments.length === 0) {
    return null;
  }

  const rootLeaf = findLeaf(`/${segments[0]}`, mode);
  if (!rootLeaf) {
    return null;
  }
  // The dashboard is what the Home crumb already links to; naming it twice
  // reads as a broken trail rather than a short one.
  if (segments.length === 1 && rootLeaf.path === "/dashboard") {
    return (
      <Breadcrumb>
        <BreadcrumbList className="text-xs sm:gap-1.5">
          <BreadcrumbItem>
            <BreadcrumbPage>{i18next.t(rootLeaf.label)}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>
    );
  }
  const rootLabel = i18next.t(rootLeaf.label);

  const lastSegment = segments[segments.length - 1];
  const lastLeaf = segments.length > 1 ? findLeaf(`/${lastSegment}`, mode) : null;
  const lastLabel = lastLeaf ? i18next.t(lastLeaf.label) : decodeURIComponent(lastSegment);

  return (
    <Breadcrumb>
      <BreadcrumbList className="text-xs sm:gap-1.5">
        <BreadcrumbItem>
          <BreadcrumbLink asChild>
            <Link to="/">{i18next.t("general:Home")}</Link>
          </BreadcrumbLink>
        </BreadcrumbItem>
        <BreadcrumbSeparator />
        {segments.length === 1 ? (
          <BreadcrumbItem>
            <BreadcrumbPage>{rootLabel}</BreadcrumbPage>
          </BreadcrumbItem>
        ) : (
          <>
            <BreadcrumbItem>
              <BreadcrumbLink asChild>
                <Link to={`/${segments[0]}`}>{rootLabel}</Link>
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage className="max-w-[240px] truncate">{lastLabel}</BreadcrumbPage>
            </BreadcrumbItem>
          </>
        )}
      </BreadcrumbList>
    </Breadcrumb>
  );
}
