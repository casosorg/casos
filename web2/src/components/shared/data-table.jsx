import * as React from "react";
import {
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import {ArrowDown, ArrowUp, ChevronLeft, ChevronRight, ChevronsUpDown, Search} from "lucide-react";
import {cn} from "@/lib/utils";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {Table, TableBody, TableCell, TableHead, TableHeader, TableRow} from "@/components/ui/table";
import {Skeleton} from "@/components/ui/skeleton";
import {EmptyState} from "@/components/shared/empty-state";

// Column descriptor accepted by DataTable:
//   {key, title, dataIndex, render(value, record, index), width, align,
//    sortable, ellipsis, className, headerClassName}
// `dataIndex` may be a dotted path. A column without one is a display column
// (actions, computed cells) and is never sortable.

function readPath(row, path) {
  if (path === undefined || path === null) {
    return undefined;
  }
  if (!String(path).includes(".")) {
    return row?.[path];
  }
  return String(path)
    .split(".")
    .reduce((acc, part) => (acc === null || acc === undefined ? acc : acc[part]), row);
}

function resolveRowKey(rowKey, record, index) {
  if (typeof rowKey === "function") {
    return rowKey(record, index);
  }
  const value = readPath(record, rowKey);
  return value === undefined || value === null ? String(index) : String(value);
}

function toColumnDefs(columns) {
  return columns.map((column, columnIndex) => {
    const id = column.key ?? column.dataIndex ?? `col-${columnIndex}`;
    const shared = {
      id: String(id),
      header: column.title,
      enableSorting: Boolean(column.sortable) && column.dataIndex !== undefined,
      meta: column,
    };

    if (column.dataIndex === undefined) {
      return {
        ...shared,
        cell: ({row}) => (column.render ? column.render(undefined, row.original, row.index) : null),
      };
    }

    return {
      ...shared,
      accessorFn: (row) => readPath(row, column.dataIndex),
      cell: ({row, getValue}) => (column.render ? column.render(getValue(), row.original, row.index) : getValue()),
    };
  });
}

function SortIcon({state}) {
  if (state === "asc") {
    return <ArrowUp className="size-3.5" />;
  }
  if (state === "desc") {
    return <ArrowDown className="size-3.5" />;
  }
  return <ChevronsUpDown className="size-3.5 opacity-40" />;
}

/**
 * The list-page workhorse. Every resource screen renders the same thing: an
 * optional toolbar, a bordered table, and pagination that only appears once the
 * data actually overflows a page.
 *
 * Props:
 *   columns, dataSource, rowKey, loading, error
 *   toolbar         node rendered on the right of the header
 *   title           heading shown on the left of the header
 *   description     small text under the title
 *   searchable      renders a filter box that matches across all cells
 *   pageSize        rows per page; pass 0 to disable pagination
 *   manualPagination controlled server-side pagination state and callbacks
 *   emptyText       shown when there is no data and nothing is loading
 *   onRowClick      makes rows interactive
 *   expandable      {rowExpandable(record), expandedRowRender(record)}
 *   dense           tighter row padding for embedded tables
 *   testId          data-testid on the root, for end-to-end tests
 *
 * The root carries data-slot and data-loading, and every row carries the
 * resolved data-row-key. Those are the selector contract the Playwright suite
 * is written against: Tailwind class names are generated and must never be
 * selected on, and a role alone cannot say *which* table or *which* row.
 */
export function DataTable({
  columns,
  dataSource,
  rowKey = "name",
  loading = false,
  toolbar,
  title,
  description,
  searchable = false,
  searchPlaceholder = "Search...",
  pageSize = 20,
  manualPagination = null,
  emptyText = "No data",
  emptyIcon,
  onRowClick,
  expandable,
  dense = false,
  className,
  tableClassName,
  testId,
}) {
  const [sorting, setSorting] = React.useState([]);
  const [globalFilter, setGlobalFilter] = React.useState("");
  const [expanded, setExpanded] = React.useState({});

  const data = React.useMemo(() => dataSource ?? [], [dataSource]);
  const columnDefs = React.useMemo(() => toColumnDefs(columns ?? []), [columns]);

  const table = useReactTable({
    data,
    columns: columnDefs,
    state: {sorting, globalFilter},
    onSortingChange: setSorting,
    onGlobalFilterChange: setGlobalFilter,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: pageSize > 0 && !manualPagination ? getPaginationRowModel() : undefined,
    getRowId: (record, index) => resolveRowKey(rowKey, record, index),
    // The default filter only sees accessor values, which would silently skip
    // display columns. Matching the whole record keeps a search for an action
    // label or a nested field working the way a reader expects.
    globalFilterFn: (row, _columnId, value) =>
      JSON.stringify(row.original ?? {})
        .toLowerCase()
        .includes(String(value).toLowerCase()),
    initialState: pageSize > 0 && !manualPagination ? {pagination: {pageSize}} : undefined,
  });

  const rows = table.getRowModel().rows;
  const showHeader = Boolean(title || description || toolbar || searchable);
  const totalRows = manualPagination?.totalRows ?? table.getFilteredRowModel().rows.length;
  const showPagination = manualPagination
    ? manualPagination.hasPreviousPage || manualPagination.hasNextPage
    : pageSize > 0 && totalRows > pageSize;

  return (
    <div
      data-slot="data-table"
      data-testid={testId}
      data-loading={loading ? "true" : "false"}
      className={cn("bg-card flex flex-col overflow-hidden rounded-xl border shadow-sm", className)}
    >
      {showHeader && (
        <div
          data-slot="data-table-header"
          className="flex flex-col gap-3 border-b px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
        >
          <div className="min-w-0">
            {title ? <h2 className="truncate text-sm font-semibold">{title}</h2> : null}
            {description ? <p className="text-muted-foreground mt-0.5 truncate text-xs">{description}</p> : null}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {searchable && (
              <div className="relative">
                <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2" />
                <Input
                  value={globalFilter}
                  onChange={(event) => setGlobalFilter(event.target.value)}
                  placeholder={searchPlaceholder}
                  className="h-8 w-44 pl-8 text-xs lg:w-56"
                />
              </div>
            )}
            {toolbar}
          </div>
        </div>
      )}

      <Table className={tableClassName} containerClassName="scrollbar-thin">
        <TableHeader className="bg-muted/40">
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id} className="hover:bg-transparent">
              {expandable ? <TableHead className="w-8" /> : null}
              {headerGroup.headers.map((header) => {
                const meta = header.column.columnDef.meta ?? {};
                const canSort = header.column.getCanSort();
                return (
                  <TableHead
                    key={header.id}
                    style={meta.width ? {width: meta.width, minWidth: meta.width} : undefined}
                    className={cn(
                      meta.align === "right" && "text-right",
                      meta.align === "center" && "text-center",
                      meta.headerClassName
                    )}
                  >
                    {canSort ? (
                      <button
                        type="button"
                        onClick={header.column.getToggleSortingHandler()}
                        className="hover:text-foreground inline-flex items-center gap-1 transition-colors"
                      >
                        {flexRender(header.column.columnDef.header, header.getContext())}
                        <SortIcon state={header.column.getIsSorted()} />
                      </button>
                    ) : (
                      flexRender(header.column.columnDef.header, header.getContext())
                    )}
                  </TableHead>
                );
              })}
            </TableRow>
          ))}
        </TableHeader>

        <TableBody>
          {loading && rows.length === 0 ? (
            Array.from({length: 5}).map((_, rowIndex) => (
              <TableRow key={`skeleton-${rowIndex}`} className="hover:bg-transparent">
                {expandable ? <TableCell /> : null}
                {columns.map((column, columnIndex) => (
                  <TableCell key={column.key ?? column.dataIndex ?? columnIndex}>
                    <Skeleton className="h-4 w-full max-w-[160px]" />
                  </TableCell>
                ))}
              </TableRow>
            ))
          ) : rows.length === 0 ? (
            <TableRow className="hover:bg-transparent">
              <TableCell colSpan={(columns?.length ?? 1) + (expandable ? 1 : 0)} className="p-0">
                <EmptyState icon={emptyIcon} title={emptyText} />
              </TableCell>
            </TableRow>
          ) : (
            rows.map((row) => {
              const isExpanded = Boolean(expanded[row.id]);
              const canExpand = expandable ? (expandable.rowExpandable ? expandable.rowExpandable(row.original) : true) : false;
              return (
                <React.Fragment key={row.id}>
                  <TableRow
                    data-row-key={row.id}
                    data-state={isExpanded ? "selected" : undefined}
                    onClick={onRowClick ? () => onRowClick(row.original) : undefined}
                    className={cn(onRowClick && "cursor-pointer")}
                  >
                    {expandable ? (
                      <TableCell className="w-8 pr-0">
                        {canExpand ? (
                          <Button
                            variant="ghost"
                            size="icon-xs"
                            aria-label={isExpanded ? "Collapse row" : "Expand row"}
                            onClick={(event) => {
                              event.stopPropagation();
                              setExpanded((prev) => ({...prev, [row.id]: !prev[row.id]}));
                            }}
                          >
                            <ChevronRight className={cn("size-3.5 transition-transform", isExpanded && "rotate-90")} />
                          </Button>
                        ) : null}
                      </TableCell>
                    ) : null}
                    {row.getVisibleCells().map((cell) => {
                      const meta = cell.column.columnDef.meta ?? {};
                      return (
                        <TableCell
                          key={cell.id}
                          style={meta.width ? {width: meta.width, minWidth: meta.width} : undefined}
                          className={cn(
                            dense && "py-1.5",
                            meta.align === "right" && "text-right",
                            meta.align === "center" && "text-center",
                            meta.ellipsis && "max-w-0 truncate",
                            meta.className
                          )}
                        >
                          {flexRender(cell.column.columnDef.cell, cell.getContext())}
                        </TableCell>
                      );
                    })}
                  </TableRow>
                  {canExpand && isExpanded ? (
                    <TableRow className="hover:bg-transparent">
                      <TableCell colSpan={(columns?.length ?? 1) + 1} className="bg-muted/30 p-0">
                        {expandable.expandedRowRender(row.original)}
                      </TableCell>
                    </TableRow>
                  ) : null}
                </React.Fragment>
              );
            })
          )}
        </TableBody>
      </Table>

      {showPagination && (
        <div className="flex items-center justify-between border-t px-4 py-2.5">
          <span className="text-muted-foreground text-xs">
            {manualPagination
              ? manualPagination.label
              : <>
                {table.getState().pagination.pageIndex * table.getState().pagination.pageSize + 1}
                {"–"}
                {Math.min((table.getState().pagination.pageIndex + 1) * table.getState().pagination.pageSize, totalRows)} of {totalRows}
              </>}
          </span>
          <div className="flex items-center gap-1">
            <Button
              variant="outline"
              size="icon-sm"
              onClick={manualPagination?.onPreviousPage ?? (() => table.previousPage())}
              disabled={manualPagination ? !manualPagination.hasPreviousPage : !table.getCanPreviousPage()}
              aria-label="Previous page"
            >
              <ChevronLeft className="size-4" />
            </Button>
            <span className="text-muted-foreground px-2 text-xs tabular-nums">
              {manualPagination ? `Page ${manualPagination.pageIndex + 1}` : `${table.getState().pagination.pageIndex + 1} / ${table.getPageCount()}`}
            </span>
            <Button
              variant="outline"
              size="icon-sm"
              onClick={manualPagination?.onNextPage ?? (() => table.nextPage())}
              disabled={manualPagination ? !manualPagination.hasNextPage : !table.getCanNextPage()}
              aria-label="Next page"
            >
              <ChevronRight className="size-4" />
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
