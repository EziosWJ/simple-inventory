import { formatDateTime } from "@/lib/datetime";
import { randomUUID } from "@/lib/uuid";
import { useCallback, useEffect, useRef, useState } from "react";
import { deleteFile, downloadFile, getFileDetail, uploadFile } from "@/api/file";
import { addPartnerAttachments, partnerAttachments, removePartnerAttachment, type PartnerAttachment } from "@/api/business";
import { Button } from "@/components/ui/button";
import { useAuthenticatedFileUrl } from "@/hooks/use-authenticated-file-url";
import { useAuthStore } from "@/store/auth-store";
import type { CurrentUserMenu } from "@/types";
import type { FileRecord } from "@/types/file";

import { emptyAttachments, type AttachmentSelection } from "@/lib/partner-attachments";
type Kind = "OPENING" | "SETTLEMENT" | "REFUND";
type Pending = { key: string; file: File; uploaded?: FileRecord; error?: string; uploading: boolean };
const permissionCode = (kind: Kind) => kind === "OPENING" ? "business:opening-balance" : kind === "REFUND" ? "business:refund" : "business:settlement";
function allowed(menus: CurrentUserMenu[], code: string): boolean {
  return menus.some(menu => menu.permissionCode === code || allowed(menu.children ?? [], code));
}
function message(error: unknown) { return error instanceof Error ? error.message : "附件操作失败"; }

export function PartnerAttachmentUpload({ kind, onChange, disabled = false, limit = 5 }: { kind: Kind; onChange: (selection: AttachmentSelection) => void; disabled?: boolean; limit?: number }) {
  const [items, setItems] = useState<Pending[]>([]);
  const [error, setError] = useState("");
  const input = useRef<HTMLInputElement>(null);
  const menus = useAuthStore(state => state.menus);
  const permitted = allowed(menus, permissionCode(kind));
  const busy = items.some(item => item.uploading);
  useEffect(() => {
    onChange({ ids: items.flatMap(item => item.uploaded ? [item.uploaded.id] : []), blocked: items.some(item => !item.uploaded), dirty: items.length > 0 });
  }, [items, onChange]);
  // Keep the temporary-file lease alive while this form remains open. Failed renewals invalidate
  // only that item; the user can retry it without losing the rest of the form.
  useEffect(() => {
    const renew = () => { for (const item of items) if (item.uploaded) void getFileDetail(item.uploaded.id).catch(e => setItems(current => current.map(row => row.key === item.key ? { ...row, uploaded: undefined, error: message(e) } : row))); };
    const timer = window.setInterval(renew, 60_000);
    const visible = () => { if (document.visibilityState === "visible") renew(); };
    document.addEventListener("visibilitychange", visible);
    return () => { clearInterval(timer); document.removeEventListener("visibilitychange", visible); };
  }, [items]);
  async function upload(item: Pending) {
    setItems(current => current.map(row => row.key === item.key ? { ...row, uploading: true, error: undefined } : row));
    try {
      if (!/\.(png|jpe?g|pdf)$/i.test(item.file.name) || item.file.size <= 0 || item.file.size > 10 * 1024 * 1024) throw new Error("仅支持 PNG、JPG、PDF，单个最多 10 MB");
      const uploaded = await uploadFile(item.file, { businessModule: `partner-attachment:${kind}` });
      setItems(current => current.map(row => row.key === item.key ? { ...row, uploaded, uploading: false } : row));
    } catch (e) { setItems(current => current.map(row => row.key === item.key ? { ...row, error: message(e), uploading: false } : row)); }
  }
  async function remove(item: Pending) {
    if (item.uploaded) {
      try { await deleteFile(item.uploaded.id); } catch (e) { setError(message(e)); return; }
    }
    setItems(current => current.filter(row => row.key !== item.key));
    setError("");
  }
  return <section className="mt-3 space-y-2 text-sm" aria-label="可选附件">
    <div className="flex flex-wrap items-center gap-3">
      <span className="font-medium">附件（选填）</span>
      <input ref={input} type="file" multiple accept=".png,.jpg,.jpeg,.pdf" aria-label="选择往来附件" className="sr-only" disabled={disabled || busy || !permitted} onChange={e => {
        const files = Array.from(e.target.files ?? []); e.target.value = "";
        if (files.length + items.length > limit) { setError(`当前最多还能选择 ${Math.max(0, limit - items.length)} 个附件`); return; }
        setError("");
        const selected = files.map(file => ({ key: randomUUID(), file, uploading: true }));
        setItems(current => [...current, ...selected]);
        for (const item of selected) void upload(item);
      }} />
      <Button type="button" size="sm" variant="secondary" disabled={disabled || busy || !permitted || items.length >= limit} onClick={() => input.current?.click()}>选择附件</Button>
      <span className="text-text-secondary">PNG、JPG、PDF · 每条最多 5 个 · 单个最多 10 MB（10 MiB）</span>
    </div>
    {!permitted && <p className="text-text-secondary">没有此业务的附件录入权限。</p>}
    <ul className="space-y-2">{items.map(item => <li key={item.key} className="flex flex-wrap items-center gap-2">
      <span className="min-w-0 break-all">{item.file.name}（{(item.file.size / 1024).toFixed(1)} KB）</span>
      <span role="status" className={item.error ? "text-error" : "text-text-secondary"}>{item.uploading ? "上传中…" : item.error ?? "上传成功"}</span>
      {item.error && <Button type="button" variant="secondary" size="sm" disabled={disabled || busy} onClick={() => void upload(item)}>重试</Button>}
      <Button type="button" variant="secondary" size="sm" disabled={disabled || busy} onClick={() => void remove(item)}>移除</Button>
    </li>)}</ul>
    {items.some(item => !item.uploaded) && <p className="text-text-secondary">附件上传完成后才能保存；也可主动移除失败项后继续。</p>}
    {error && <p role="alert" className="text-error">{error}</p>}
  </section>;
}

function AttachmentFile({ item }: { item: PartnerAttachment }) {
  const [preview, setPreview] = useState(false);
  const [error, setError] = useState("");
  const { url, loading, error: previewError } = useAuthenticatedFileUrl(preview ? `/api/system/file/${item.fileId}/view` : null);
  async function download() {
    try {
      const blob = await downloadFile(item.fileId); const objectUrl = URL.createObjectURL(blob);
      const link = document.createElement("a"); link.href = objectUrl; link.download = item.originalName; link.click();
      window.setTimeout(() => URL.revokeObjectURL(objectUrl), 1000); setError("");
    } catch (e) { setError(message(e)); }
  }
  return <div className="min-w-0 space-y-2">
    <div className="flex flex-wrap items-center gap-2"><span className="break-all">{item.originalName}</span><Button size="sm" variant="secondary" onClick={() => setPreview(value => !value)}>{preview ? "收起预览" : "预览"}</Button><Button size="sm" variant="secondary" onClick={() => void download()}>下载</Button></div>
    {loading && <p role="status">加载预览中…</p>}{(error || previewError) && <p role="alert" className="text-error">{error || previewError}</p>}
    {url && (item.mimeType === "application/pdf" ? <iframe title={item.originalName} src={url} className="h-96 w-full rounded-control border border-border" /> : <a href={url} target="_blank" rel="noreferrer" title="打开原图放大查看"><img src={url} alt={item.originalName} className="max-h-96 max-w-full object-contain" /></a>)}
  </div>;
}

export function PartnerAttachmentDetails({ entryId, kind, readOnly, onChanged }: { entryId: number; kind: Kind; readOnly: boolean; onChanged: () => void }) {
  const [items, setItems] = useState<PartnerAttachment[]>([]);
  const [selection, setSelection] = useState(emptyAttachments);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [revision, setRevision] = useState(0);
  const [ready, setReady] = useState(false);
  const menus = useAuthStore(state => state.menus);
  const permitted = allowed(menus, permissionCode(kind));
  const load = useCallback(async () => { setReady(false); try { setItems(await partnerAttachments(entryId)); setError(""); setReady(true); } catch (e) { setError(message(e)); } }, [entryId]);
  useEffect(() => { void load(); }, [load]);
  async function mutate(action: () => Promise<unknown>) {
    setBusy(true); setError("");
    try { await action(); setSelection(emptyAttachments); setRevision(value => value + 1); await load(); onChanged(); }
    catch (e) { setError(message(e)); }
    finally { setBusy(false); }
  }
  const active = items.filter(item => !item.removedAt);
  const removed = items.filter(item => item.removedAt);
  return <section className="space-y-3 border-t border-border pt-3" aria-label="往来附件详情">
    <h3 className="font-medium">附件（{active.length}）</h3>
    {!ready && !error && <p role="status">读取附件中…</p>}
    {ready && active.length === 0 && <p className="text-text-secondary">暂无附件</p>}
    {active.map(item => <div key={item.id} className="space-y-2"><AttachmentFile item={item} /><p className="text-text-secondary">上传：{item.uploadedName || item.uploadedBy} · {formatDateTime(item.uploadedAt)}</p>{!readOnly && permitted && <Button size="sm" variant="secondary" disabled={busy || selection.dirty} onClick={() => { if (window.confirm("移除此附件？原文件将保留在附件历史中。")) void mutate(() => removePartnerAttachment(entryId, item.id)); }}>移除附件</Button>}</div>)}
    {readOnly ? <p className="text-text-secondary">已冲销，附件和历史只读。</p> : permitted && ready && <><PartnerAttachmentUpload key={`${entryId}-${revision}`} kind={kind} limit={5 - active.length} disabled={busy} onChange={setSelection} /><Button size="sm" disabled={busy || selection.blocked || selection.ids.length === 0} onClick={() => void mutate(() => addPartnerAttachments(entryId, selection.ids))}>保存补传附件</Button></>}
    <details><summary className="cursor-pointer">附件历史（已移除 {removed.length}）</summary><div className="mt-3 space-y-3">{removed.map(item => <div key={item.id}><AttachmentFile item={item} /><p className="text-text-secondary">上传：{item.uploadedName || item.uploadedBy} · {formatDateTime(item.uploadedAt)}；移除：{item.removedName || item.removedBy} · {formatDateTime(item.removedAt!)}</p></div>)}{removed.length === 0 && <p className="text-text-secondary">暂无移除记录</p>}</div></details>
    {error && <p role="alert" className="text-error">{error}</p>}
  </section>;
}
