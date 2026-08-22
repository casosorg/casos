import React, {useState} from "react";
import {Tabs, TabsContent, TabsList, TabsTrigger} from "@/components/ui/tabs";

/**
 * The shell behind simple mode's combined pages. Each tab renders one of the
 * advanced list pages unchanged, so "Storage" is the PVC page and the storage
 * class page under one plain-language heading rather than a reimplementation of
 * either. The children keep their own PageContainer padding, which is why the
 * heading only pads itself.
 */
export function TabPage({title, description, tabs}) {
  const [active, setActive] = useState(tabs[0]?.value);
  const current = tabs.find((tab) => tab.value === active) ?? tabs[0];

  return (
    <Tabs value={active} onValueChange={setActive} className="min-w-0 flex-1 gap-0">
      <div className="flex flex-col gap-3 px-4 pt-4 md:px-6 md:pt-6">
        <div className="min-w-0">
          <h1 className="truncate text-xl font-semibold tracking-tight">{title}</h1>
          {description ? <p className="text-muted-foreground mt-1 text-sm">{description}</p> : null}
        </div>
        {tabs.length > 1 ? (
          <TabsList>
            {tabs.map((tab) => (
              <TabsTrigger key={tab.value} value={tab.value} className="px-3">
                {tab.icon ? <tab.icon /> : null}
                {tab.label}
              </TabsTrigger>
            ))}
          </TabsList>
        ) : null}
        {current?.hint ? <p className="text-muted-foreground text-sm">{current.hint}</p> : null}
      </div>

      {tabs.map((tab) => (
        <TabsContent key={tab.value} value={tab.value} className="min-w-0">
          {tab.content}
        </TabsContent>
      ))}
    </Tabs>
  );
}
