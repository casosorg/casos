import React, {Suspense, lazy} from "react";
import {Route, Switch} from "react-router-dom";
import {Button} from "@/components/ui/button";
import {Loading} from "@/components/shared/loading";
import {ResultScreen} from "@/components/shared/misc";

// Every route is code-split: the charting and terminal pages pull in libraries a
// reader who only visits the pod list should never download.
const DashboardPage = lazy(() => import("@/pages/DashboardPage"));
const NamespaceListPage = lazy(() => import("@/pages/NamespaceListPage"));
const ConfigMapListPage = lazy(() => import("@/pages/ConfigMapListPage"));
const SecretListPage = lazy(() => import("@/pages/SecretListPage"));
const ServiceAccountListPage = lazy(() => import("@/pages/ServiceAccountListPage"));
const ClusterRoleBindingListPage = lazy(() => import("@/pages/ClusterRoleBindingListPage"));
const RoleBindingListPage = lazy(() => import("@/pages/RoleBindingListPage"));
const PvcListPage = lazy(() => import("@/pages/PvcListPage"));
const StorageClassListPage = lazy(() => import("@/pages/StorageClassListPage"));
const ResourceQuotaListPage = lazy(() => import("@/pages/ResourceQuotaListPage"));
const HPAListPage = lazy(() => import("@/pages/HPAListPage"));
const ServiceListPage = lazy(() => import("@/pages/ServiceListPage"));
const NetworkPolicyListPage = lazy(() => import("@/pages/NetworkPolicyListPage"));
const IngressListPage = lazy(() => import("@/pages/IngressListPage"));
const NodeListPage = lazy(() => import("@/pages/NodeListPage"));
const AdmissionPolicyPage = lazy(() => import("@/pages/AdmissionPolicyPage"));
const AuthorizationPolicyPage = lazy(() => import("@/pages/AuthorizationPolicyPage"));
const TrivyScanPage = lazy(() => import("@/pages/TrivyScanPage"));
const DaemonSetListPage = lazy(() => import("@/pages/DaemonSetListPage"));
const StatefulSetListPage = lazy(() => import("@/pages/StatefulSetListPage"));
const JobListPage = lazy(() => import("@/pages/JobListPage"));
const CronJobListPage = lazy(() => import("@/pages/CronJobListPage"));
const PodListPage = lazy(() => import("@/pages/PodListPage"));
const DeploymentListPage = lazy(() => import("@/pages/DeploymentListPage"));
const LogSearchPage = lazy(() => import("@/pages/LogSearchPage"));
const SiteListPage = lazy(() => import("@/pages/SiteListPage"));
const SiteEditPage = lazy(() => import("@/pages/SiteEditPage"));
const MachineListPage = lazy(() => import("@/pages/MachineListPage"));
const MachineEditPage = lazy(() => import("@/pages/MachineEditPage"));
const TopologyPage = lazy(() => import("@/pages/TopologyPage"));
const AppStorePage = lazy(() => import("@/pages/AppStorePage"));
const HelmReleasePage = lazy(() => import("@/pages/HelmReleasePage"));
const MonitorPage = lazy(() => import("@/pages/MonitorPage"));

function NotFound() {
  return (
    <ResultScreen
      status="404"
      title="404 NOT FOUND"
      subTitle="Sorry, the page you visited does not exist."
      extra={
        <Button asChild>
          <a href="/">Back Home</a>
        </Button>
      }
    />
  );
}

export function AppRoutes({account, accountUpdatedAt, onOpenAccount, onUpdateSite}) {
  return (
    <Suspense fallback={<Loading type="page" />}>
      <Switch>
        <Route
          exact
          path={["/", "/dashboard"]}
          render={(props) => <DashboardPage account={account} accountUpdatedAt={accountUpdatedAt} onOpenAccount={onOpenAccount} {...props} />}
        />
        <Route exact path="/namespaces" component={NamespaceListPage} />
        <Route exact path="/configmaps" component={ConfigMapListPage} />
        <Route exact path="/secrets" component={SecretListPage} />
        <Route exact path="/serviceaccounts" component={ServiceAccountListPage} />
        <Route exact path="/clusterrolebindings" component={ClusterRoleBindingListPage} />
        <Route exact path="/rolebindings" component={RoleBindingListPage} />
        <Route exact path="/pvcs" component={PvcListPage} />
        <Route exact path="/storageclasses" component={StorageClassListPage} />
        <Route exact path="/resourcequotas" component={ResourceQuotaListPage} />
        <Route exact path="/hpas" component={HPAListPage} />
        <Route exact path="/services" component={ServiceListPage} />
        <Route exact path="/networkpolicies" component={NetworkPolicyListPage} />
        <Route exact path="/ingresses" component={IngressListPage} />
        <Route exact path="/nodes" component={NodeListPage} />
        <Route exact path="/admission-policy" component={AdmissionPolicyPage} />
        <Route exact path="/authorization-policy" component={AuthorizationPolicyPage} />
        <Route exact path="/trivy-scans" component={TrivyScanPage} />
        <Route exact path="/daemonsets" component={DaemonSetListPage} />
        <Route exact path="/statefulsets" component={StatefulSetListPage} />
        <Route exact path="/jobs" component={JobListPage} />
        <Route exact path="/cronjobs" component={CronJobListPage} />
        <Route exact path="/pods" component={PodListPage} />
        <Route exact path="/deployments" component={DeploymentListPage} />
        <Route exact path="/log-search" component={LogSearchPage} />
        <Route exact path="/sites" component={SiteListPage} />
        <Route exact path="/sites/:siteName" render={(props) => <SiteEditPage onUpdateSite={onUpdateSite} {...props} />} />
        <Route exact path="/machines" render={(props) => <MachineListPage account={account} {...props} />} />
        <Route exact path="/machines/:machineName" component={MachineEditPage} />
        <Route exact path="/topology" component={TopologyPage} />
        <Route exact path="/app-store" component={AppStorePage} />
        <Route exact path="/helm-releases" component={HelmReleasePage} />
        <Route exact path="/monitor" component={MonitorPage} />
        <Route path="" component={NotFound} />
      </Switch>
    </Suspense>
  );
}
