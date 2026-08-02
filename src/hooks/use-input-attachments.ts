import * as React from "react"

import {
  deleteInputAttachment,
  uploadTaskAttachment,
} from "@/lib/api"
import type { InputAttachment } from "@/types"

const MAX_INPUT_ATTACHMENT_SIZE = 20 * 1024 * 1024 * 1024
const MAX_INPUT_ATTACHMENTS = 20

type UploadState = "uploading" | "error" | "done" | "processing"

export interface PendingInputAttachment {
  clientId: string
  file: File
  progress: number
  state: UploadState
  attachment?: InputAttachment
  error?: string
}

export function useInputAttachments() {
  const scopeKey = "task"
  const [bucket, setBucket] = React.useState<{
    key: string
    items: PendingInputAttachment[]
  }>({ key: scopeKey, items: [] })
  const items = React.useMemo(
    () => (bucket.key === scopeKey ? bucket.items : []),
    [bucket, scopeKey]
  )
  const itemsRef = React.useRef(items)
  const scopeKeyRef = React.useRef(scopeKey)
  const boundAttachmentIDsRef = React.useRef(new Set<string>())
  React.useEffect(() => {
    itemsRef.current = items
  }, [items])
  React.useEffect(() => {
    scopeKeyRef.current = scopeKey
    const boundAttachmentIDs = boundAttachmentIDsRef.current
    return () => {
      if (scopeKeyRef.current === scopeKey) scopeKeyRef.current = ""
      for (const item of itemsRef.current) {
        if (item.attachment && !boundAttachmentIDs.has(item.attachment.id)) {
          void deleteInputAttachment(item.attachment.id)
        }
      }
    }
  }, [scopeKey])

  const setItems = React.useCallback(
    (
      update:
        | PendingInputAttachment[]
        | ((items: PendingInputAttachment[]) => PendingInputAttachment[])
    ) => {
      setBucket((current) => {
        const currentItems = current.key === scopeKey ? current.items : []
        return {
          key: scopeKey,
          items: typeof update === "function" ? update(currentItems) : update,
        }
      })
    },
    [scopeKey]
  )

  const updateItem = React.useCallback(
    (clientId: string, update: Partial<PendingInputAttachment>) => {
      setItems((current) =>
        current.map((item) =>
          item.clientId === clientId ? { ...item, ...update } : item
        )
      )
    },
    [setItems]
  )

  const addFiles = React.useCallback(
    (files: FileList | File[]) => {
      const uploadKey = scopeKeyRef.current
      const remaining = Math.max(
        0,
        MAX_INPUT_ATTACHMENTS - itemsRef.current.length
      )
      const additions = Array.from(files)
        .slice(0, remaining)
        .map((file) => ({
          clientId: crypto.randomUUID(),
          file,
          progress: 0,
          state: (file.size > MAX_INPUT_ATTACHMENT_SIZE
            ? "error"
            : "uploading") as UploadState,
          error:
            file.size > MAX_INPUT_ATTACHMENT_SIZE
              ? "单个附件不能超过 20 GiB"
              : undefined,
        }))
      setItems((current) => [...current, ...additions])
      for (const item of additions) {
        if (item.state === "error") continue
        void uploadTaskAttachment(item.file, (progress) => {
          if (scopeKeyRef.current === uploadKey) {
            updateItem(item.clientId, { progress })
          }
        })
          .then((attachment) => {
            if (scopeKeyRef.current === uploadKey) {
              updateItem(item.clientId, {
                attachment,
                progress: 100,
                state: "done",
              })
            } else {
              void deleteInputAttachment(attachment.id)
            }
          })
          .catch((reason) => {
            if (scopeKeyRef.current === uploadKey) {
              updateItem(item.clientId, {
                state: "error",
                error:
                  reason instanceof Error ? reason.message : "附件上传失败",
              })
            }
          })
      }
    },
    [setItems, updateItem]
  )

  const remove = React.useCallback(
    async (clientId: string) => {
      const item = itemsRef.current.find(
        (candidate) => candidate.clientId === clientId
      )
      if (!item) return
      if (!item.attachment) {
        setItems((current) =>
          current.filter((candidate) => candidate.clientId !== clientId)
        )
        return
      }
      updateItem(clientId, { state: "processing" })
      try {
        await deleteInputAttachment(item.attachment.id)
        setItems((current) =>
          current.filter((candidate) => candidate.clientId !== clientId)
        )
      } catch (reason) {
        updateItem(clientId, {
          state: "error",
          error: reason instanceof Error ? reason.message : "删除附件失败",
        })
      }
    },
    [setItems, updateItem]
  )

  const clearBound = React.useCallback(() => {
    for (const item of itemsRef.current) {
      if (item.attachment) boundAttachmentIDsRef.current.add(item.attachment.id)
    }
    setItems([])
  }, [setItems])

  return {
    items,
    addFiles,
    remove,
    clearBound,
    attachmentIds: items.flatMap((item) =>
      item.state === "done" && item.attachment ? [item.attachment.id] : []
    ),
    uploading: items.some(
      (item) => item.state === "uploading" || item.state === "processing"
    ),
    hasErrors: items.some((item) => item.state === "error"),
  }
}
