import { useEffect, useState } from "react";
import { getPrintProfile, updatePrintProfile, type PrintProfile } from "@/api/print-profile";
import { ContentCard } from "@/components/common/content-card";
import { Field } from "@/components/common/field";
import { PageHeader } from "@/components/common/page-header";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { getErrorMessage } from "@/lib/api-error";

const emptyProfile: PrintProfile = { name: "", phone: "", address: "" };

export function PrintProfilePage() {
  const [profile, setProfile] = useState(emptyProfile);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    getPrintProfile()
      .then(setProfile)
      .catch((reason: unknown) => setError(getErrorMessage(reason, "读取经营者打印资料失败")))
      .finally(() => setLoading(false));
  }, []);

  async function save() {
    setSaving(true);
    setError("");
    try {
      const updated = await updatePrintProfile(profile);
      setProfile(updated);
      toast.success("经营者打印资料已保存");
    } catch (reason) {
      const message = getErrorMessage(reason, "保存失败，请检查资料后重试");
      setError(message);
      toast.error(message);
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader title="经营者打印资料" description="维护后续送货单使用的经营者名称、电话和地址。已保存的业务单据保留过账时的资料。" />
      <ContentCard>
        <div className="max-w-2xl space-y-5">
          <Field label="经营者名称" required>
            <Input maxLength={200} value={profile.name} onChange={(event) => setProfile({ ...profile, name: event.target.value })} placeholder="请输入经营者或店铺名称" disabled={loading} />
          </Field>
          <Field label="联系电话" help="选填，最多 50 个字符">
            <Input maxLength={50} value={profile.phone} onChange={(event) => setProfile({ ...profile, phone: event.target.value })} disabled={loading} />
          </Field>
          <Field label="经营地址" help="选填，最多 500 个字符">
            <Textarea maxLength={500} rows={3} value={profile.address} onChange={(event) => setProfile({ ...profile, address: event.target.value })} disabled={loading} />
          </Field>
          {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
          <div className="flex justify-end">
            <Button onClick={save} disabled={loading || saving}>{saving ? "保存中…" : "保存资料"}</Button>
          </div>
        </div>
      </ContentCard>
    </div>
  );
}
