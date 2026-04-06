import * as React from "react";
import { Tabs as TabsPrimitive } from "@base-ui/react/tabs";

import { cn } from "@/lib/utils";

function Tabs({ className, ...props }: TabsPrimitive.Root.Props) {
  return (
    <TabsPrimitive.Root
      data-slot="tabs"
      className={cn("flex flex-col gap-6", className)}
      {...props}
    />
  );
}

function TabList({ className, ...props }: TabsPrimitive.List.Props) {
  return (
    <TabsPrimitive.List
      data-slot="tab-list"
      className={cn(
        "flex gap-1 border-b border-border",
        className,
      )}
      {...props}
    />
  );
}

function Tab({ className, ...props }: TabsPrimitive.Tab.Props) {
  return (
    <TabsPrimitive.Tab
      data-slot="tab"
      className={cn(
        "-mb-px cursor-pointer border-b-2 border-transparent px-3 pb-2.5 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground data-[selected]:border-primary data-[selected]:text-foreground",
        className,
      )}
      {...props}
    />
  );
}

function TabPanel({ className, ...props }: TabsPrimitive.Panel.Props) {
  return (
    <TabsPrimitive.Panel
      data-slot="tab-panel"
      className={cn("outline-none", className)}
      {...props}
    />
  );
}

export { Tabs, TabList, Tab, TabPanel };
