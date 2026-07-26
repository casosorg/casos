import React, {useCallback, useEffect, useRef, useState} from "react";
import {Alert, Button, Card, Col, Input, Pagination, Row, Spin, Tabs, Tag, Typography} from "antd";
import {ReloadOutlined, RocketOutlined, SearchOutlined} from "@ant-design/icons";
import {useTranslation} from "react-i18next";
import * as AppBackend from "./backend/AppBackend";
import DeployAppModal from "./DeployAppModal";

const {Title, Paragraph, Text} = Typography;

function isHelmTemplate(template) {
  return template?.packageType === "helm";
}

function getSourceLabel(template, t) {
  if (template?.source === "artifacthub") {return "ArtifactHub";}
  if (template?.source === "sealos") {return "Sealos";}
  if (template?.source === "repository") {return t("appStore:Custom repository");}
  return template?.source || "";
}

function AppCard({template, onDeploy}) {
  const {t} = useTranslation();
  const [imgErr, setImgErr] = useState(false);
  const category = template.categories?.find(c => c && c !== "Helm") ?? "";
  const sourceLabel = getSourceLabel(template, t);
  const isHelm = isHelmTemplate(template);

  return (
    <Card
      hoverable
      styles={{body: {padding: "16px"}}}
      style={{height: "100%", display: "flex", flexDirection: "column"}}
    >
      <div style={{display: "flex", alignItems: "flex-start", gap: 12}}>
        <div style={{
          width: 48, height: 48, flexShrink: 0,
          display: "flex", alignItems: "center", justifyContent: "center",
          borderRadius: 10, background: "#f5f5f5", overflow: "hidden",
        }}>
          {!imgErr && template.icon ? (
            <img
              src={template.icon}
              alt={template.title}
              style={{width: 36, height: 36, objectFit: "contain"}}
              onError={() => setImgErr(true)}
            />
          ) : (
            <Text strong style={{fontSize: 20}}>{(template.title || "?")[0]}</Text>
          )}
        </div>
        <div style={{flex: 1, minWidth: 0}}>
          <div style={{display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap"}}>
            <Text strong style={{fontSize: 15}}>{template.title}</Text>
            {sourceLabel && <Tag color="blue" style={{margin: 0}}>{sourceLabel}</Tag>}
            {isHelm && <Tag color="green" style={{margin: 0}}>{t("appStore:Helm")}</Tag>}
            {category && <Tag style={{margin: 0}}>{category}</Tag>}
          </div>
          <Paragraph
            ellipsis={{rows: 2}}
            style={{marginTop: 4, marginBottom: 0, fontSize: 13, color: "rgba(0,0,0,0.55)"}}
          >
            {template.description || ""}
          </Paragraph>
        </div>
      </div>

      {isHelm && (template.repoName || template.chartName || template.version) && (
        <div style={{marginTop: 10, fontSize: 12, color: "rgba(0,0,0,0.55)"}}>
          {template.repoName && <Text type="secondary">{template.repoName}</Text>}
          {template.chartName && <Text code style={{marginLeft: template.repoName ? 6 : 0}}>{template.chartName}</Text>}
          {template.version && <Tag style={{marginLeft: 6, fontSize: 11}}>v{template.version}</Tag>}
        </div>
      )}

      {template.ports?.length > 0 && (
        <div style={{marginTop: 10, display: "flex", gap: 4, flexWrap: "wrap"}}>
          {template.ports.map(p => (
            <Tag key={p} style={{fontSize: 11}}>:{p}</Tag>
          ))}
        </div>
      )}

      <div style={{marginTop: 12, textAlign: "right"}}>
        <Button type="primary" size="small" icon={<RocketOutlined />} onClick={() => onDeploy(template)}>
          {t("appStore:Deploy")}
        </Button>
      </div>
    </Card>
  );
}

function templateKey(tpl) {
  return `${tpl.source || "app"}-${tpl.repoUrl || ""}-${tpl.chartName || tpl.name}-${tpl.version || ""}`;
}

function CardGrid({items, onDeploy, emptyText}) {
  return (
    <Row gutter={[16, 16]}>
      {items.map(tpl => (
        <Col key={templateKey(tpl)} xs={24} sm={12} lg={8} xl={6}>
          <AppCard template={tpl} onDeploy={onDeploy} />
        </Col>
      ))}
      {items.length === 0 && (
        <Col span={24}>
          <Paragraph style={{color: "rgba(0,0,0,0.4)", textAlign: "center", padding: "48px 0"}}>
            {emptyText}
          </Paragraph>
        </Col>
      )}
    </Row>
  );
}

// helmItemToTemplate converts a backend HelmChartListItem into the template
// shape shared with DeployAppModal.
function helmItemToTemplate(item) {
  return {
    name: item.chartName,
    title: item.title || item.chartName,
    description: item.description,
    icon: item.icon,
    categories: item.categories ?? [],
    source: item.source,
    packageType: item.packageType || "helm",
    repoName: item.repoName,
    repoUrl: item.repoUrl,
    chartName: item.chartName,
    version: item.version,
  };
}

function FeaturedTab({onDeploy}) {
  const {t} = useTranslation();
  const [templates, setTemplates] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [search, setSearch] = useState("");
  const [activeCategory, setActiveCategory] = useState(null);

  const fetchTemplates = useCallback(() => {
    setLoading(true);
    setError(null);
    AppBackend.getAppTemplates()
      .then(res => {
        if (res.status === "ok") {
          setTemplates(res.data ?? []);
        } else {
          setError(res.msg);
        }
      })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    fetchTemplates();
  }, [fetchTemplates]);

  const allCategories = [...new Set(
    templates.flatMap(tp => tp.categories ?? []).filter(Boolean)
  )].sort();

  const filtered = templates.filter(tpl => {
    const keyword = search.toLowerCase();
    const matchSearch = !search
      || tpl.title?.toLowerCase().includes(keyword)
      || tpl.description?.toLowerCase().includes(keyword)
      || tpl.name?.toLowerCase().includes(keyword)
      || tpl.chartName?.toLowerCase().includes(keyword)
      || tpl.repoName?.toLowerCase().includes(keyword);
    const matchCat = !activeCategory || (tpl.categories ?? []).includes(activeCategory);
    return matchSearch && matchCat;
  });

  if (error) {
    return (
      <Alert
        type="error"
        message={t("appStore:Load failed")}
        description={error}
        showIcon
        action={
          <Button size="small" icon={<ReloadOutlined />} onClick={fetchTemplates}>
            {t("appStore:Retry")}
          </Button>
        }
      />
    );
  }

  return (
    <>
      <div style={{display: "flex", gap: 12, marginBottom: 16, flexWrap: "wrap", alignItems: "center"}}>
        <Input.Search
          placeholder={t("appStore:Search placeholder")}
          value={search}
          onChange={e => setSearch(e.target.value)}
          style={{width: 220}}
          allowClear
        />
        {allCategories.length > 0 && (
          <div style={{display: "flex", gap: 6, flexWrap: "wrap"}}>
            <Tag
              color={!activeCategory ? "blue" : "default"}
              style={{cursor: "pointer", padding: "2px 10px"}}
              onClick={() => setActiveCategory(null)}
            >
              {t("appStore:All")}
            </Tag>
            {allCategories.map(cat => (
              <Tag
                key={cat}
                color={activeCategory === cat ? "blue" : "default"}
                style={{cursor: "pointer", padding: "2px 10px"}}
                onClick={() => setActiveCategory(activeCategory === cat ? null : cat)}
              >
                {cat}
              </Tag>
            ))}
          </div>
        )}
        <Button size="small" icon={<ReloadOutlined />} onClick={fetchTemplates} loading={loading}>
          {t("general:Refresh")}
        </Button>
      </div>

      {loading ? (
        <div style={{textAlign: "center", padding: "80px 0"}}>
          <Spin size="large" />
          <div style={{marginTop: 16, color: "rgba(0,0,0,0.45)"}}>{t("appStore:Loading")}</div>
        </div>
      ) : (
        <CardGrid items={filtered} onDeploy={onDeploy} emptyText={t("appStore:No results")} />
      )}
    </>
  );
}

function ArtifactHubTab({onDeploy}) {
  const {t} = useTranslation();
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize] = useState(24);
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const debounceRef = useRef(null);

  const fetchCharts = useCallback((q, p) => {
    setLoading(true);
    setError(null);
    AppBackend.searchHelmCharts(q, p, pageSize)
      .then(res => {
        if (res.status === "ok") {
          setItems(res.data?.items ?? []);
          setTotal(res.data?.total ?? 0);
        } else {
          setError(res.msg);
        }
      })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, [pageSize]);

  useEffect(() => {
    fetchCharts("", 1);
  }, [fetchCharts]);

  const handleQueryChange = value => {
    setQuery(value);
    setPage(1);
    if (debounceRef.current) {
      clearTimeout(debounceRef.current);
    }
    debounceRef.current = setTimeout(() => fetchCharts(value, 1), 400);
  };

  const handlePageChange = p => {
    setPage(p);
    fetchCharts(query, p);
  };

  return (
    <>
      <div style={{display: "flex", gap: 12, marginBottom: 16, flexWrap: "wrap", alignItems: "center"}}>
        <Input
          prefix={<SearchOutlined />}
          placeholder={t("appStore:Search all charts")}
          value={query}
          onChange={e => handleQueryChange(e.target.value)}
          style={{width: 280}}
          allowClear
        />
        {total > 0 && (
          <Text type="secondary">{t("appStore:chart count", {count: total})}</Text>
        )}
      </div>

      {error && <Alert type="error" message={error} showIcon style={{marginBottom: 16}} />}

      <Spin spinning={loading}>
        <CardGrid
          items={items.map(helmItemToTemplate)}
          onDeploy={onDeploy}
          emptyText={t("appStore:No results")}
        />
      </Spin>

      {total > pageSize && (
        <div style={{marginTop: 16, textAlign: "center"}}>
          <Pagination
            current={page}
            pageSize={pageSize}
            total={total}
            showSizeChanger={false}
            onChange={handlePageChange}
          />
        </div>
      )}
    </>
  );
}

function CustomRepoTab({onDeploy}) {
  const {t} = useTranslation();
  const [repoUrl, setRepoUrl] = useState("");
  const [items, setItems] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [loaded, setLoaded] = useState(false);
  const [search, setSearch] = useState("");

  const fetchRepo = () => {
    const trimmed = repoUrl.trim();
    if (!trimmed) {return;}
    setLoading(true);
    setError(null);
    AppBackend.getHelmRepoCharts(trimmed)
      .then(res => {
        if (res.status === "ok") {
          setItems(res.data ?? []);
          setLoaded(true);
        } else {
          setError(res.msg);
        }
      })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  };

  const filtered = items.filter(item => {
    if (!search) {return true;}
    const keyword = search.toLowerCase();
    return item.name?.toLowerCase().includes(keyword)
      || item.description?.toLowerCase().includes(keyword);
  });

  return (
    <>
      <Alert
        type="info"
        showIcon
        style={{marginBottom: 16}}
        message={t("appStore:Custom repository desc")}
      />
      <div style={{display: "flex", gap: 12, marginBottom: 16, flexWrap: "wrap", alignItems: "center"}}>
        <Input
          placeholder="https://charts.example.com"
          value={repoUrl}
          onChange={e => setRepoUrl(e.target.value)}
          onPressEnter={fetchRepo}
          style={{width: 360}}
          allowClear
        />
        <Button type="primary" onClick={fetchRepo} loading={loading}>
          {t("appStore:Load repository")}
        </Button>
        {loaded && (
          <Input.Search
            placeholder={t("appStore:Search placeholder")}
            value={search}
            onChange={e => setSearch(e.target.value)}
            style={{width: 200}}
            allowClear
          />
        )}
      </div>

      {error && <Alert type="error" message={error} showIcon style={{marginBottom: 16}} />}

      <Spin spinning={loading}>
        {loaded && (
          <CardGrid
            items={filtered.map(helmItemToTemplate)}
            onDeploy={onDeploy}
            emptyText={t("appStore:No results")}
          />
        )}
      </Spin>
    </>
  );
}

function AppStorePage() {
  const {t} = useTranslation();
  const [deployTarget, setDeployTarget] = useState(null);

  const onDeploy = tpl => setDeployTarget(tpl);

  return (
    <div style={{padding: "24px"}}>
      <div style={{marginBottom: 12}}>
        <Title level={4} style={{marginBottom: 4}}>{t("general:App Store")}</Title>
        <Paragraph style={{marginBottom: 0, color: "rgba(0,0,0,0.55)"}}>
          {t("appStore:App Store desc")}
        </Paragraph>
      </div>

      <Tabs
        defaultActiveKey="featured"
        items={[
          {
            key: "featured",
            label: t("appStore:Featured"),
            children: <FeaturedTab onDeploy={onDeploy} />,
          },
          {
            key: "artifacthub",
            label: "ArtifactHub",
            children: <ArtifactHubTab onDeploy={onDeploy} />,
          },
          {
            key: "custom",
            label: t("appStore:Custom repository"),
            children: <CustomRepoTab onDeploy={onDeploy} />,
          },
        ]}
      />

      <DeployAppModal
        open={!!deployTarget}
        template={deployTarget}
        onClose={() => setDeployTarget(null)}
      />
    </div>
  );
}

export default AppStorePage;
