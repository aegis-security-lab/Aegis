import * as React from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"
import {
  createKnowledgeBase,
  updateKnowledgeBase,
  type SaveKnowledgeBaseInput,
} from "@/lib/api"
import type { KnowledgeBase } from "@/types"

export function KnowledgeBaseDialog({
  knowledgeBase,
  onOpenChange,
  onSaved,
}: {
  knowledgeBase: KnowledgeBase | null
  onOpenChange: (open: boolean) => void
  onSaved: (knowledgeBase: KnowledgeBase) => void | Promise<void>
}) {
  const [form, setForm] = React.useState<SaveKnowledgeBaseInput>(() => ({
    name: knowledgeBase?.name ?? "",
    description: knowledgeBase?.description ?? "",
    retrievalProvider: knowledgeBase?.retrievalProvider ?? "keyword_ai",
  }))
  const [saving, setSaving] = React.useState(false)

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!form.name.trim() || !form.description.trim() || saving) return
    setSaving(true)
    try {
      const saved = knowledgeBase
        ? await updateKnowledgeBase(knowledgeBase.id, form)
        : await createKnowledgeBase(form)
      toast.success(knowledgeBase ? "知识库已更新" : "知识库已创建")
      await onSaved(saved)
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "保存知识库失败")
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <form onSubmit={submit} className="contents">
          <DialogHeader>
            <DialogTitle>
              {knowledgeBase ? "编辑知识库" : "创建知识库"}
            </DialogTitle>
            <DialogDescription>
              介绍会随关联关系注入 Agent 的系统上下文，帮助它判断何时检索。
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="knowledge-base-name">名称</FieldLabel>
              <Input
                id="knowledge-base-name"
                autoFocus
                maxLength={100}
                placeholder="例如：支付域设计规范"
                value={form.name}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    name: event.target.value,
                  }))
                }
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="knowledge-base-description">
                知识库介绍
              </FieldLabel>
              <Textarea
                id="knowledge-base-description"
                rows={5}
                maxLength={5000}
                placeholder="说明这里包含什么知识、适合解决哪些问题，以及不包含什么。"
                value={form.description}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    description: event.target.value,
                  }))
                }
              />
              <FieldDescription>
                Agent 在启动 Session 时就能看到这段介绍，但文档内容只在调用检索工具后返回。
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="knowledge-provider">检索引擎</FieldLabel>
              <Select
                value={form.retrievalProvider}
                onValueChange={(value) =>
                  setForm((current) => ({
                    ...current,
                    retrievalProvider: String(
                      value
                    ) as SaveKnowledgeBaseInput["retrievalProvider"],
                  }))
                }
              >
                <SelectTrigger id="knowledge-provider" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value="keyword_ai">关键词 + AI</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
              <FieldDescription>
                先用关键词从完整文档召回候选，再由只读检索 Agent
                根据每篇文档开头最多 1000 字符重排。
              </FieldDescription>
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              取消
            </Button>
            <Button
              type="submit"
              disabled={
                saving || !form.name.trim() || !form.description.trim()
              }
            >
              {saving ? <Spinner data-icon="inline-start" /> : null}
              {saving ? "保存中…" : "保存"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
