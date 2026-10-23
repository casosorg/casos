import React from "react";
import {useHistory} from "react-router-dom";
import {useTranslation} from "react-i18next";
import {Cpu, RefreshCw, Sparkles, Zap} from "lucide-react";
import * as AiModelBackend from "@/backend/AiModelBackend";
import * as Setting from "@/Setting";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle} from "@/components/ui/card";
import {Tabs, TabsContent, TabsList, TabsTrigger} from "@/components/ui/tabs";
import {CodeBlock, CodeText} from "@/components/shared/misc";
import {useResource} from "@/hooks/use-resource";
import {useUiMode} from "@/hooks/use-ui-mode";
import {formatBytes} from "@/lib/quantity";

const TOKEN_PLACEHOLDER = "<your-token>";

const modelApiUrl = () => `${Setting.ServerUrl || window.location.origin}/v1`;

const snippets = [
  {
    key: "python",
    title: "Python",
    code: (url, model) => [
      "from openai import OpenAI",
      "",
      `client = OpenAI(base_url="${url}", api_key="${TOKEN_PLACEHOLDER}")`,
      "reply = client.chat.completions.create(",
      `    model="${model}",`,
      "    messages=[{\"role\": \"user\", \"content\": \"Hello!\"}],",
      ")",
      "print(reply.choices[0].message.content)",
    ].join("\n"),
  },
  {
    key: "javascript",
    title: "JavaScript",
    code: (url, model) => [
      "import OpenAI from \"openai\";",
      "",
      `const client = new OpenAI({baseURL: "${url}", apiKey: "${TOKEN_PLACEHOLDER}"});`,
      "const reply = await client.chat.completions.create({",
      `  model: "${model}",`,
      "  messages: [{role: \"user\", content: \"Hello!\"}],",
      "});",
      "console.log(reply.choices[0].message.content);",
    ].join("\n"),
  },
  {
    key: "curl",
    title: "curl",
    code: (url, model) => [
      `curl ${url}/chat/completions \\`,
      `  -H "Authorization: Bearer ${TOKEN_PLACEHOLDER}" \\`,
      "  -H \"Content-Type: application/json\" \\",
      `  -d '{"model": "${model}", "messages": [{"role": "user", "content": "Hello!"}]}'`,
    ].join("\n"),
  },
];

/**
 * Shows the OpenAI-compatible API casos serves for the Ollama models in the
 * cluster: where to point an SDK, which models answer, and on which GPUs.
 */
export function LocalModelsCard() {
  const {t} = useTranslation();
  const history = useHistory();
  const {resolvePath} = useUiMode();
  const {data: models, loading, refresh} = useResource(() => AiModelBackend.getAiModels(), [], {initialData: []});
  const url = modelApiUrl();
  const chatModel = (models ?? []).find((model) => !model.name.includes("embed"))?.name ?? "qwen3:8b";
  const onLocalhost = ["localhost", "127.0.0.1", "[::1]"].includes(window.location.hostname);

  return (
    <Card data-testid="local-models-card">
      <CardHeader>
        <CardTitle>{t("agent:Use your local models from any app")}</CardTitle>
        <CardDescription>
          {t("agent:casos serves an OpenAI-compatible API for the models on your GPUs. Use a full-access token below as the API key.")}
        </CardDescription>
        <CardAction>
          <Button variant="outline" size="sm" onClick={() => refresh()} loading={loading}>
            <RefreshCw />
            {t("general:Refresh")}
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <span className="font-medium">{t("agent:Base URL")}</span>
            <CodeText copyable>{url}</CodeText>
          </div>
          {onLocalhost ? (
            <p className="text-muted-foreground text-sm">{t("agent:From another computer, use this machine's network address instead of localhost.")}</p>
          ) : null}
        </div>
        {!loading && (models ?? []).length === 0 ? (
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-dashed p-4">
            <p className="text-muted-foreground text-sm">
              {t("agent:No models yet. Install Private AI to run a model on your GPU, and it shows up here.")}
            </p>
            <Button size="sm" onClick={() => history.push(resolvePath("/app-store"))}>
              <Sparkles />
              {t("agent:Open the App Store")}
            </Button>
          </div>
        ) : (
          <ul className="divide-y rounded-md border" data-testid="local-models-list">
            {(models ?? []).map((model) => (
              <li key={model.name} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 text-sm">
                <CodeText copyable>{model.name}</CodeText>
                <span className="text-muted-foreground">
                  {[model.parameterSize, model.quantization, model.size ? formatBytes(model.size) : ""].filter(Boolean).join(" · ")}
                </span>
                <span className="ml-auto flex flex-wrap gap-1">
                  {model.machines.map((machine, index) => (
                    <Badge key={`${machine}-${index}`} variant={model.gpus[index] ? "success" : "muted"} title={machine}>
                      {model.gpus[index] ? <Zap /> : <Cpu />}
                      {model.gpus[index] ? model.gpus[index].replace(/^NVIDIA /, "") : machine}
                    </Badge>
                  ))}
                </span>
              </li>
            ))}
          </ul>
        )}
        {(models ?? []).some((model) => model.machines.length > 1) ? (
          <p className="text-muted-foreground text-sm">
            {t("agent:A model on several machines answers from the least busy one.")}
          </p>
        ) : null}
        <Tabs defaultValue={snippets[0].key}>
          <TabsList>
            {snippets.map((snippet) => (
              <TabsTrigger key={snippet.key} value={snippet.key}>{snippet.title}</TabsTrigger>
            ))}
          </TabsList>
          {snippets.map((snippet) => (
            <TabsContent key={snippet.key} value={snippet.key}>
              <CodeBlock copyable>{snippet.code(url, chatModel)}</CodeBlock>
            </TabsContent>
          ))}
        </Tabs>
      </CardContent>
    </Card>
  );
}
