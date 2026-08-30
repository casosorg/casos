import React, {useCallback, useEffect, useMemo, useState} from "react";
import i18next from "i18next";
import {Bell, CheckCheck, ChevronRight, CircleAlert, TriangleAlert} from "lucide-react";
import * as EventBackend from "@/backend/EventBackend";
import {Popover, PopoverContent, PopoverTrigger} from "@/components/ui/popover";
import {Tabs, TabsContent, TabsList, TabsTrigger} from "@/components/ui/tabs";
import {useResource} from "@/hooks/use-resource";
import {cn} from "@/lib/utils";
import {appKeyForKind} from "@/desktop/registry";

const READ_KEY = "desktopReadNotifications";

/** Enough history that a read notification cannot come back; not enough to grow forever. */
const READ_LIMIT = 500;

const EVENT_POLL_INTERVAL = 30000;

function readSeen() {
  try {
    const parsed = JSON.parse(localStorage.getItem(READ_KEY) ?? "[]");
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function writeSeen(ids) {
  try {
    localStorage.setItem(READ_KEY, JSON.stringify(ids.slice(-READ_LIMIT)));
  } catch {
    // A desktop that cannot remember what was read is still usable.
  }
}

function since(timestamp) {
  const at = Date.parse(`${timestamp}Z`);
  if (Number.isNaN(at)) {
    return "";
  }
  const seconds = Math.max(0, Math.round((Date.now() - at) / 1000));
  if (seconds < 60) {
    return i18next.t("desktop:just now");
  }
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) {
    return i18next.t("desktop:{{count}}m ago", {count: minutes});
  }
  const hours = Math.round(minutes / 60);
  if (hours < 24) {
    return i18next.t("desktop:{{count}}h ago", {count: hours});
  }
  return i18next.t("desktop:{{count}}d ago", {count: Math.round(hours / 24)});
}

/**
 * Everything the cluster has to say, as one list.
 *
 * Warnings the API server recorded are the substance; the two derived lines are
 * the standing conditions that no single event describes.
 */
function buildNotifications(events, stats) {
  const notifications = (events ?? []).map((event) => ({
    id: `event:${event.namespace}/${event.name}:${event.lastSeen}`,
    severity: event.type === "Warning" ? "warning" : "info",
    title: `${event.reason}: ${event.objectName}`,
    body: event.message,
    context: `${event.namespace || "cluster"} - ${event.kind}`,
    at: event.lastSeen,
    kind: event.kind,
  }));

  (stats?.unhealthyPods ?? []).forEach((pod) => {
    notifications.unshift({
      id: `pod:${pod.namespace}/${pod.name}:${pod.reason ?? pod.status}`,
      severity: "error",
      title: pod.name,
      body: pod.reason ?? pod.status,
      context: pod.namespace,
      at: "",
      kind: "Pod",
    });
  });

  const notReady = Math.max((stats?.nodesTotal ?? 0) - (stats?.nodesReady ?? 0), 0);
  if (notReady > 0) {
    notifications.unshift({
      id: `nodes-not-ready:${notReady}`,
      severity: "error",
      title: i18next.t("desktop:Nodes not ready"),
      body: `${notReady} / ${stats?.nodesTotal ?? 0}`,
      context: i18next.t("general:Nodes"),
      at: "",
      kind: "Node",
    });
  }

  return notifications;
}

function SeverityIcon({severity}) {
  if (severity === "error") {
    return <CircleAlert className="text-destructive mt-0.5 size-4 shrink-0" />;
  }
  if (severity === "warning") {
    return <TriangleAlert className="text-warning mt-0.5 size-4 shrink-0" />;
  }
  return <Bell className="text-muted-foreground mt-0.5 size-4 shrink-0" />;
}

function NotificationList({notifications, empty, onOpen}) {
  if (notifications.length === 0) {
    return <p className="text-muted-foreground px-2 py-8 text-center text-sm">{empty}</p>;
  }
  return (
    <div className="max-h-80 overflow-auto p-1.5">
      {notifications.map((notification) => {
        const target = appKeyForKind(notification.kind);
        const body = (
          <>
            <SeverityIcon severity={notification.severity} />
            <div className="min-w-0 flex-1">
              <p className="truncate font-medium">{notification.title}</p>
              <p className="text-muted-foreground line-clamp-2 text-xs">{notification.body}</p>
              <p className="text-muted-foreground/80 mt-0.5 text-[11px]">
                {notification.context}
                {notification.at ? ` - ${since(notification.at)}` : ""}
              </p>
            </div>
            {target ? <ChevronRight className="text-muted-foreground mt-0.5 size-4 shrink-0" /> : null}
          </>
        );
        const className = "flex w-full items-start gap-2 rounded-md px-2 py-2 text-left text-sm";

        // A notification about something with a page of its own opens it; one
        // about a kind the desktop has no app for is not a false promise.
        return target ? (
          <button key={notification.id} type="button" className={cn(className, "hover:bg-accent/50")} onClick={() => onOpen(notification, target)}>
            {body}
          </button>
        ) : (
          <div key={notification.id} className={className}>{body}</div>
        );
      })}
    </div>
  );
}

/**
 * The desktop's notification centre. Unread is the default view because the
 * only reason to open a bell is to find out what has not been seen yet.
 */
export function NotificationCenter({stats, onOpenApp}) {
  const [seen, setSeen] = useState(readSeen);
  const [open, setOpen] = useState(false);

  const {data: events} = useResource(() => EventBackend.getEvents("all", "Warning", 50), [], {
    initialData: [],
    toastOnError: false,
    pollInterval: EVENT_POLL_INTERVAL,
  });

  const notifications = useMemo(() => buildNotifications(events, stats), [events, stats]);
  const seenSet = useMemo(() => new Set(seen), [seen]);
  const unread = useMemo(() => notifications.filter((item) => !seenSet.has(item.id)), [notifications, seenSet]);

  const markAllRead = useCallback(() => {
    setSeen((current) => {
      const next = Array.from(new Set([...current, ...notifications.map((item) => item.id)]));
      writeSeen(next);
      return next;
    });
  }, [notifications]);

  // Opening the panel is the act of reading it, but only what was on screen at
  // that moment: anything arriving later stays unread.
  useEffect(() => {
    if (!open) {
      return undefined;
    }
    const timer = setTimeout(markAllRead, 1500);
    return () => clearTimeout(timer);
  }, [open, markAllRead]);

  function openTarget(notification, appKey) {
    setOpen(false);
    onOpenApp?.(appKey, notification);
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          data-testid="desktop-notifications"
          aria-label={i18next.t("desktop:Notifications")}
          className="relative flex size-8 items-center justify-center rounded-lg text-white/90 hover:bg-white/15"
        >
          <Bell className="size-4" />
          {unread.length > 0 && (
            <span className="bg-destructive absolute -top-0.5 -right-0.5 flex h-4 min-w-4 items-center justify-center rounded-full px-1 text-[10px] font-semibold text-white">
              {unread.length > 99 ? "99+" : unread.length}
            </span>
          )}
        </button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-96 p-0">
        <div className="flex items-center justify-between border-b px-3 py-2">
          <span className="text-sm font-medium">{i18next.t("desktop:Notifications")}</span>
          <button
            type="button"
            onClick={markAllRead}
            disabled={unread.length === 0}
            className="text-muted-foreground hover:text-foreground flex items-center gap-1 text-xs disabled:opacity-40"
          >
            <CheckCheck className="size-3.5" />
            {i18next.t("desktop:Mark all read")}
          </button>
        </div>
        <Tabs defaultValue="unread">
          <TabsList className="m-1.5 grid w-[calc(100%-0.75rem)] grid-cols-2">
            <TabsTrigger value="unread">
              {i18next.t("desktop:Unread")}
              {unread.length > 0 ? ` (${unread.length})` : ""}
            </TabsTrigger>
            <TabsTrigger value="all">{i18next.t("general:All")}</TabsTrigger>
          </TabsList>
          <TabsContent value="unread">
            <NotificationList notifications={unread} empty={i18next.t("desktop:Nothing needs attention")} onOpen={openTarget} />
          </TabsContent>
          <TabsContent value="all">
            <NotificationList notifications={notifications} empty={i18next.t("desktop:Nothing needs attention")} onOpen={openTarget} />
          </TabsContent>
        </Tabs>
      </PopoverContent>
    </Popover>
  );
}

export default NotificationCenter;
