import React, {useState} from "react";
import i18next from "i18next";
import {useTranslation} from "react-i18next";
import {KeyRound, Plus, RefreshCw, Trash2} from "lucide-react";
import * as AccessTokenBackend from "@/backend/AccessTokenBackend";
import * as Setting from "@/Setting";
import {runAction, useResource} from "@/hooks/use-resource";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {Alert, AlertDescription, AlertTitle} from "@/components/ui/alert";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "@/components/ui/card";
import {Tabs, TabsContent, TabsList, TabsTrigger} from "@/components/ui/tabs";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {DataTable} from "@/components/shared/data-table";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {SimpleSelect} from "@/components/shared/simple-select";
import {LocalModelsCard} from "@/components/shared/local-models-card";
import {PageContainer, PageHeader} from "@/components/shared/page-header";
import {CodeBlock, CodeText} from "@/components/shared/misc";

const TOKEN_PLACEHOLDER = "<your-token>";

const mcpUrl = () => `${Setting.ServerUrl || window.location.origin}/mcp`;

const clients = [
  {
    key: "claude-code",
    title: () => "Claude Code",
    hint: "agent:Run once in a terminal.",
    snippet: (url, token) => `claude mcp add --transport http --scope user casos ${url} --header "Authorization: Bearer ${token}"`,
  },
  {
    key: "cursor",
    title: () => "Cursor",
    hint: "agent:Add to ~/.cursor/mcp.json, or .cursor/mcp.json in a project.",
    snippet: (url, token) => JSON.stringify({mcpServers: {casos: {url, headers: {Authorization: `Bearer ${token}`}}}}, null, 2),
  },
  {
    key: "codex",
    title: () => "Codex",
    hint: "agent:Add to ~/.codex/config.toml.",
    snippet: (url, token) => [
      "[mcp_servers.casos]",
      `url = "${url}"`,
      `http_headers = { "Authorization" = "Bearer ${token}" }`,
    ].join("\n"),
  },
  {
    key: "other",
    title: () => i18next.t("agent:Other clients"),
    hint: "agent:Any MCP client that speaks Streamable HTTP works. Point it at the URL and send the token as a bearer token.",
    snippet: (url, token) => JSON.stringify({type: "http", url, headers: {Authorization: `Bearer ${token}`}}, null, 2),
  },
];

const examplePrompts = [
  ["agent:Deploy {{image}} to my casos as \"shop\" on port 3000", {image: "ghcr.io/me/shop:1.4"}],
  ["agent:Why is shop failing on casos? Show me its logs"],
  ["agent:Roll shop back to the previous version"],
];

function ClientSetup({token}) {
  const url = mcpUrl();
  return (
    <Tabs defaultValue={clients[0].key}>
      <TabsList>
        {clients.map((client) => (
          <TabsTrigger key={client.key} value={client.key}>
            {client.title()}
          </TabsTrigger>
        ))}
      </TabsList>
      {clients.map((client) => (
        <TabsContent key={client.key} value={client.key} className="flex flex-col gap-2">
          <p className="text-muted-foreground text-sm">{i18next.t(client.hint)}</p>
          <CodeBlock copyable>{client.snippet(url, token || TOKEN_PLACEHOLDER)}</CodeBlock>
        </TabsContent>
      ))}
    </Tabs>
  );
}

/**
 * Connects AI coding agents to casos over MCP. An agent authenticates with an
 * access token created here, and from then on can deploy an image, read its
 * logs and roll it back on the user's behalf.
 */
function AgentAccessPage() {
  useTranslation();
  const {data: tokens, loading, refresh} = useResource(() => AccessTokenBackend.getAccessTokens(), [], {initialData: []});

  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [readOnly, setReadOnly] = useState(true);
  const [nameError, setNameError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [created, setCreated] = useState(null);

  function openCreate() {
    setName((tokens ?? []).length === 0 ? "my-agent" : "");
    setNameError("");
    setReadOnly(true);
    setCreateOpen(true);
  }

  async function handleCreate() {
    if (!name.trim()) {
      setNameError(i18next.t("agent:Name is required"));
      return;
    }
    setSubmitting(true);
    const ok = await runAction(AccessTokenBackend.addAccessToken(name.trim(), readOnly), {
      onSuccess: (res) => setCreated(res.data),
    });
    setSubmitting(false);
    if (ok) {
      setCreateOpen(false);
      refresh();
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(AccessTokenBackend.deleteAccessToken(record.name), {
      successMessage: i18next.t("agent:Token revoked"),
    });
    if (ok) {
      refresh();
    }
  }

  const columns = [
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", sortable: true, className: "font-medium"},
    {
      key: "prefix",
      title: i18next.t("agent:Token"),
      dataIndex: "prefix",
      render: (value) => <CodeText>{`${value}…`}</CodeText>,
    },
    {
      key: "readOnly",
      title: i18next.t("agent:Permissions"),
      dataIndex: "readOnly",
      render: (value) => i18next.t(value ? "agent:Read-only" : "agent:Full access"),
    },
    {key: "createdTime", title: i18next.t("general:Created"), dataIndex: "createdTime", width: 200, sortable: true},
    {
      key: "lastUsedTime",
      title: i18next.t("agent:Last used"),
      dataIndex: "lastUsedTime",
      width: 200,
      sortable: true,
      render: (value) => value || <span className="text-muted-foreground">{i18next.t("agent:Never")}</span>,
    },
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 100,
      align: "right",
      render: (_, record) => (
        <ConfirmDialog
          title={i18next.t("agent:Revoke this token?")}
          description={i18next.t("agent:Agents using it lose access to casos at once.")}
          confirmText={i18next.t("agent:Revoke")}
          onConfirm={() => handleDelete(record)}
        >
          <Button variant="outline" size="sm" className="text-destructive" aria-label={i18next.t("agent:Revoke")}>
            <Trash2 />
          </Button>
        </ConfirmDialog>
      ),
    },
  ];

  return (
    <PageContainer>
      <PageHeader
        title={i18next.t("agent:AI Agents")}
        description={i18next.t("agent:Let Claude Code, Cursor, Codex and other MCP clients deploy to casos, read app logs, roll back, and run code in sandboxes on your own machines.")}
      />

      <Card>
        <CardHeader>
          <CardTitle>{i18next.t("agent:Connect an agent")}</CardTitle>
          <CardDescription>
            {i18next.t("agent:Create a token below, then add this address to your agent as an MCP server")}{" "}
            <CodeText copyable>{mcpUrl()}</CodeText>
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <ClientSetup />
          <div className="flex flex-col gap-1.5">
            <p className="text-sm font-medium">{i18next.t("agent:Then just ask")}</p>
            <ul className="text-muted-foreground list-disc pl-5 text-sm">
              {examplePrompts.map(([key, values]) => <li key={key}>{i18next.t(key, values)}</li>)}
            </ul>
            <p className="text-muted-foreground text-sm">
              {i18next.t("agent:The agent builds and pushes an image of your project, then casos pulls it, so use a registry your cluster can reach.")}
            </p>
          </div>
        </CardContent>
      </Card>

      <LocalModelsCard />

      <DataTable
        title={i18next.t("agent:Access tokens")}
        description={i18next.t("agent:A token acts as you. Give each agent or machine its own, and revoke the ones you no longer use.")}
        columns={columns}
        dataSource={tokens}
        rowKey={(record) => record.name}
        loading={loading}
        emptyText={i18next.t("agent:No tokens yet")}
        toolbar={
          <>
            <Button variant="outline" size="sm" onClick={() => refresh()} loading={loading}>
              <RefreshCw />
              {i18next.t("general:Refresh")}
            </Button>
            <Button size="sm" onClick={openCreate}>
              <Plus />
              {i18next.t("agent:Create token")}
            </Button>
          </>
        }
      />

      <FormDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        title={i18next.t("agent:Create token")}
        submitText={i18next.t("general:Create")}
        cancelText={i18next.t("general:Cancel")}
        submitting={submitting}
        onSubmit={handleCreate}
      >
        <Field label={i18next.t("general:Name")} htmlFor="access-token-name" required error={nameError}>
          <Input
            id="access-token-name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="claude-code-laptop"
            autoComplete="off"
            autoFocus
          />
        </Field>
        <Field label={i18next.t("agent:Permissions")} htmlFor="access-token-permissions">
          <SimpleSelect
            id="access-token-permissions"
            value={readOnly ? "read-only" : "full"}
            onChange={(value) => setReadOnly(value === "read-only")}
            options={[
              {value: "read-only", label: i18next.t("agent:Read-only")},
              {value: "full", label: i18next.t("agent:Full access")},
            ]}
          />
          <p className="text-muted-foreground text-sm">
            {i18next.t(readOnly
              ? "agent:Read app status, logs and diagnostics. Cannot deploy, execute commands, access sandboxes or use the model gateway."
              : "agent:Allow all MCP tools and the model gateway, including deployment, deletion and command execution.")}
          </p>
        </Field>
      </FormDialog>

      <FormDialog
        open={Boolean(created)}
        onOpenChange={(open) => !open && setCreated(null)}
        title={i18next.t("agent:Token created")}
        size="lg"
        footer={
          <Button type="button" onClick={() => setCreated(null)}>{i18next.t("general:Done")}</Button>
        }
      >
        {created ? (
          <div className="flex min-w-0 flex-col gap-4">
            <Alert>
              <KeyRound />
              <AlertTitle>{i18next.t("agent:Copy it now")}</AlertTitle>
              <AlertDescription>{i18next.t("agent:This is the only time casos shows the token. Store it like a password.")}</AlertDescription>
            </Alert>
            <CodeBlock copyable className="break-all">{created.secret}</CodeBlock>
            <ClientSetup token={created.secret} />
          </div>
        ) : null}
      </FormDialog>
    </PageContainer>
  );
}

export default AgentAccessPage;
