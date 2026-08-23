import React from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Label,
  LabelList,
  Pie,
  PieChart,
  PolarAngleAxis,
  PolarRadiusAxis,
  RadialBar,
  RadialBarChart,
  XAxis,
  YAxis,
} from "recharts";
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart";
import {cn} from "@/lib/utils";

// The theme owns the palette. Every mark drawn here cycles through the five
// --chart-* slots, which are defined once per theme, so a chart follows a light
// or dark switch instead of carrying colours of its own.
const CHART_COLORS = [
  "var(--chart-1)",
  "var(--chart-2)",
  "var(--chart-3)",
  "var(--chart-4)",
  "var(--chart-5)",
];

function truncate(value, max) {
  const text = String(value ?? "");
  return text.length > max ? `${text.slice(0, max - 1)}…` : text;
}

function toEntries(data) {
  return Object.entries(data || {}).map(([name, value]) => ({name, value}));
}

// The palette lives in the config rather than on the marks so the legend and
// the tooltip can look a series' colour up by name.
function paletteConfig(entries) {
  return Object.fromEntries(entries.map(({name}, index) => [
    name,
    {label: name, color: CHART_COLORS[index % CHART_COLORS.length]},
  ]));
}

// Pie tooltips carry the share as well as the count, which is the number the
// donut is actually drawn from.
function shareFormatter(total) {
  return (value, name, item) => (
    <>
      <div
        className="h-2.5 w-2.5 shrink-0 rounded-[2px]"
        style={{backgroundColor: item?.payload?.fill || item?.color}}
      />
      <div className="flex flex-1 items-center justify-between gap-4 leading-none">
        <span className="text-muted-foreground">{name}</span>
        <span className="text-foreground font-mono font-medium tabular-nums">
          {value}{total > 0 ? ` (${((value / total) * 100).toFixed(1)}%)` : ""}
        </span>
      </div>
    </>
  );
}

function CenteredTotal({value, label}) {
  return ({viewBox}) => {
    if (!viewBox || !("cx" in viewBox)) {
      return null;
    }
    return (
      <text x={viewBox.cx} y={viewBox.cy} textAnchor="middle" dominantBaseline="middle">
        <tspan x={viewBox.cx} y={viewBox.cy} className="fill-foreground text-2xl font-semibold tabular-nums">
          {value}
        </tspan>
        <tspan x={viewBox.cx} y={(viewBox.cy || 0) + 22} className="fill-muted-foreground text-xs">
          {label}
        </tspan>
      </text>
    );
  };
}

/**
 * Category breakdown as a donut with the population in the middle and the
 * legend underneath, so the card reads as one number plus its split.
 */
export function CategoryDonut({data, centerLabel, className}) {
  const plain = toEntries(data);
  if (plain.length === 0) {
    return null;
  }
  const config = paletteConfig(plain);
  const total = plain.reduce((sum, entry) => sum + entry.value, 0);
  // The slice colour rides on the datum rather than on a <Cell> child, which
  // leaves <Label> as the pie's only child and the centre total free to use it.
  const entries = plain.map((entry) => ({...entry, fill: config[entry.name].color}));

  return (
    <ChartContainer config={config} className={cn("mx-auto aspect-square", className)}>
      <PieChart>
        <ChartTooltip
          cursor={false}
          content={<ChartTooltipContent hideLabel nameKey="name" formatter={shareFormatter(total)} />}
        />
        <Pie
          data={entries}
          dataKey="value"
          nameKey="name"
          innerRadius="58%"
          outerRadius="82%"
          stroke="var(--card)"
          strokeWidth={2}
        >
          {centerLabel ? <Label content={CenteredTotal({value: total, label: centerLabel})} /> : null}
        </Pie>
        <ChartLegend
          content={<ChartLegendContent nameKey="name" />}
          className="mt-1 flex-wrap gap-x-4 gap-y-1.5"
        />
      </PieChart>
    </ChartContainer>
  );
}

/**
 * Horizontal bars for a "top N" ranking. A cluster can have hundreds of
 * namespaces, so the caller passes the slice worth looking at. One bar colour
 * throughout: the categories are a ranking, not five separate series.
 */
export function RankedBarChart({data, limit = 10, valueLabel, className}) {
  const rows = toEntries(data)
    .sort((a, b) => b.value - a.value)
    .slice(0, limit);
  if (rows.length === 0) {
    return null;
  }
  const config = {value: {label: valueLabel, color: "var(--chart-1)"}};

  return (
    <ChartContainer config={config} className={className}>
      <BarChart accessibilityLayer data={rows} layout="vertical" margin={{left: 4, right: 32}}>
        <CartesianGrid horizontal={false} />
        <YAxis
          dataKey="name"
          type="category"
          width={132}
          tickLine={false}
          axisLine={false}
          tickMargin={8}
          tickFormatter={(value) => truncate(value, 18)}
        />
        <XAxis dataKey="value" type="number" hide />
        <ChartTooltip cursor={false} content={<ChartTooltipContent indicator="line" />} />
        <Bar dataKey="value" fill="var(--color-value)" radius={4} maxBarSize={28}>
          <LabelList
            dataKey="value"
            position="right"
            offset={8}
            className="fill-muted-foreground"
            fontSize={12}
          />
        </Bar>
      </BarChart>
    </ChartContainer>
  );
}

/**
 * Single-percentage gauge. The muted track behind the arc is the background
 * sector recharts draws, which the chart container already themes.
 */
export function RadialGauge({value = 0, label, caption, color = "var(--chart-1)", className}) {
  const percent = Math.min(100, Math.max(0, Number(value) || 0));
  const config = {value: {label: label ?? "", color}};

  return (
    <ChartContainer config={config} className={cn("mx-auto aspect-square", className)}>
      <RadialBarChart
        data={[{name: "value", value: percent}]}
        startAngle={90}
        endAngle={-270}
        innerRadius="68%"
        outerRadius="100%"
      >
        <PolarAngleAxis type="number" domain={[0, 100]} angleAxisId={0} tick={false} />
        <RadialBar dataKey="value" angleAxisId={0} background cornerRadius={8} fill={color} />
        <PolarRadiusAxis tick={false} tickLine={false} axisLine={false}>
          <Label content={CenteredTotal({value: `${percent}%`, label: caption})} />
        </PolarRadiusAxis>
      </RadialBarChart>
    </ChartContainer>
  );
}
