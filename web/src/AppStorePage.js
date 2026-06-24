import React, {useEffect, useState} from "react";
import {Alert, Button, Card, Col, Input, Row, Spin, Tag, Typography} from "antd";
import {ReloadOutlined, RocketOutlined} from "@ant-design/icons";
import {useTranslation} from "react-i18next";
import * as AppBackend from "./backend/AppBackend";
import DeployAppModal from "./DeployAppModal";

const {Title, Paragraph, Text} = Typography;

function AppCard({template, onDeploy}) {
  const {t} = useTranslation();
  const [imgErr, setImgErr] = useState(false);
  const category = template.categories?.[0] ?? "";

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

function AppStorePage() {
  const {t} = useTranslation();
  const [templates, setTemplates] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [search, setSearch] = useState("");
  const [activeCategory, setActiveCategory] = useState(null);
  const [deployTarget, setDeployTarget] = useState(null);

  const fetchTemplates = () => {
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
  };

  useEffect(() => {
    fetchTemplates();
  }, []);

  const allCategories = [...new Set(
    templates.flatMap(t => t.categories ?? []).filter(Boolean)
  )].sort();

  const filtered = templates.filter(tpl => {
    const matchSearch = !search
      || tpl.title?.toLowerCase().includes(search.toLowerCase())
      || tpl.description?.toLowerCase().includes(search.toLowerCase())
      || tpl.name?.toLowerCase().includes(search.toLowerCase());
    const matchCat = !activeCategory || (tpl.categories ?? []).includes(activeCategory);
    return matchSearch && matchCat;
  });

  return (
    <div style={{padding: "24px"}}>
      <div style={{marginBottom: 20}}>
        <Title level={4} style={{marginBottom: 4}}>{t("general:App Store")}</Title>
        <Paragraph style={{marginBottom: 0, color: "rgba(0,0,0,0.55)"}}>
          {t("appStore:App Store desc")}
          {!loading && templates.length > 0 && (
            <span> {t("appStore:app count", {count: templates.length})}</span>
          )}
        </Paragraph>
      </div>

      {error && (
        <Alert
          type="error"
          message={t("appStore:Load failed")}
          description={error}
          showIcon
          style={{marginBottom: 16}}
          action={
            <Button size="small" icon={<ReloadOutlined />} onClick={fetchTemplates}>
              {t("appStore:Retry")}
            </Button>
          }
        />
      )}

      {!error && (
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
      )}

      {loading ? (
        <div style={{textAlign: "center", padding: "80px 0"}}>
          <Spin size="large" />
          <div style={{marginTop: 16, color: "rgba(0,0,0,0.45)"}}>
            {t("appStore:Loading")}
          </div>
        </div>
      ) : (
        <Row gutter={[16, 16]}>
          {filtered.map(tpl => (
            <Col key={tpl.name} xs={24} sm={12} lg={8} xl={6}>
              <AppCard template={tpl} onDeploy={t2 => setDeployTarget(t2)} />
            </Col>
          ))}
          {filtered.length === 0 && (
            <Col span={24}>
              <Paragraph style={{color: "rgba(0,0,0,0.4)", textAlign: "center", padding: "48px 0"}}>
                {t("appStore:No results")}
              </Paragraph>
            </Col>
          )}
        </Row>
      )}

      <DeployAppModal
        open={!!deployTarget}
        template={deployTarget}
        onClose={() => setDeployTarget(null)}
      />
    </div>
  );
}

export default AppStorePage;
