import React from "react";
import {Breadcrumb} from "antd";
import {Link} from "react-router-dom";
import i18next from "i18next";

const RESOURCE_LABELS = {
  "dashboard": "general:Dashboard",
  "deployments": "general:Deployments",
  "pods": "general:Pods",
  "nodes": "general:Nodes",
  "namespaces": "general:Namespaces",
  "serviceaccounts": "general:ServiceAccounts",
  "configmaps": "general:ConfigMaps",
  "secrets": "general:Secrets",
  "services": "general:Services",
  "clusterrolebindings": "general:ClusterRoleBindings",
  "ingresses": "general:Ingresses",
  "statefulsets": "general:Stateful Sets",
  "sites": "general:Sites",
  "machines": "general:Machines",
  "monitor": "general:Monitor Center",
};

function buildBreadcrumbItems(uri) {
  const pathSegments = (uri || "").split("/").filter(Boolean);
  const homeItem = {title: <Link to="/">Home</Link>};

  if (pathSegments.length === 0) {return null;}

  const rootSegment = pathSegments[0];
  const listLabelKey = RESOURCE_LABELS[rootSegment];
  if (!listLabelKey) {return null;}

  const label = i18next.t(listLabelKey);

  if (pathSegments.length === 1) {
    return [homeItem, {title: label}];
  }

  const lastSegment = pathSegments[pathSegments.length - 1];
  const lastLabelKey = RESOURCE_LABELS[lastSegment];
  const lastLabel = lastLabelKey ? i18next.t(lastLabelKey) : decodeURIComponent(lastSegment);

  return [
    homeItem,
    {title: <Link to={`/${rootSegment}`}>{label}</Link>},
    {title: lastLabel},
  ];
}

const BreadcrumbBar = ({uri}) => {
  const items = buildBreadcrumbItems(uri);
  if (!items) {return null;}
  return <Breadcrumb items={items} style={{marginLeft: 8}} />;
};

export default BreadcrumbBar;
