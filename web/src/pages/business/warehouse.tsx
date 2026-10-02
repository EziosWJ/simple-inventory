import { useEffect, useState } from "react";
import { getWarehouse, saveWarehouse } from "@/api/business";
import { BusinessLeaveConfirm } from "@/components/business/business-leave-confirm";
import { ContentCard } from "@/components/common/content-card";
import { PageHeader } from "@/components/common/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useBusinessLeaveGuard } from "@/hooks/use-business-leave-guard";

export function WarehousePage() {
  const [name, setName] = useState("");
  const [remark, setRemark] = useState("");
  const [baseline, setBaseline] = useState<{ name: string; remark: string } | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const dirty = baseline !== null && (name !== baseline.name || remark !== baseline.remark);
  const leaveGuard = useBusinessLeaveGuard({ dirty, busy: saving });

  useEffect(() => {
    let active = true;
    void getWarehouse().then((warehouse) => {
      if (!active) return;
      const next = { name: warehouse.name, remark: warehouse.remark ?? "" };
      setName(next.name);
      setRemark(next.remark);
      setBaseline(next);
    }).catch((reason: unknown) => {
      if (!active) return;
      setError(reason instanceof Error ? reason.message : "加载失败");
      setBaseline({ name: "", remark: "" });
    }).finally(() => {
      if (active) setLoading(false);
    });
    return () => { active = false; };
  }, []);

  async function save(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setError("");
    setSaved(false);
    try {
      const next = { name, remark };
      await saveWarehouse({ name, remark: remark || null });
      setBaseline(next);
      setSaved(true);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "保存失败");
    } finally {
      setSaving(false);
    }
  }

  return <>
    <PageHeader title="仓库" description="系统使用唯一逻辑仓库作为库存归属。" />
    <ContentCard className="max-w-2xl">
      <form className="grid gap-4" onSubmit={save}>
        <label className="grid gap-1 text-sm">仓库名称<Input maxLength={100} value={name} onChange={event => setName(event.target.value)} required disabled={loading || saving} /></label>
        <label className="grid gap-1 text-sm">备注<Input maxLength={500} value={remark} onChange={event => setRemark(event.target.value)} disabled={loading || saving} /></label>
        {error && <p role="alert" className="text-sm text-danger">{error}</p>}
        {saved && <p role="status" className="text-sm text-success">已保存</p>}
        <div><Button type="submit" disabled={loading || saving}>{saving ? "保存中…" : "保存"}</Button></div>
      </form>
    </ContentCard>
    <BusinessLeaveConfirm guard={leaveGuard} title="离开仓库设置" stayText="继续编辑" leaveText="放弃并离开" />
  </>;
}
