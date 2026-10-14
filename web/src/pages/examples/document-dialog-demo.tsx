import { useState } from "react";
import {
  DocumentDisclosure,
  DocumentItems,
  DocumentOperationRecords,
  DocumentSummary,
  type DocumentItem,
} from "@/components/business/document-presentation";
import { ContentCard } from "@/components/common/content-card";
import { DetailDialog } from "@/components/common/detail-dialog";
import { Field } from "@/components/common/field";
import { FormDialog } from "@/components/common/form-dialog";
import { PageHeader } from "@/components/common/page-header";
import { StatusTag } from "@/components/common/status-tag";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";

const DOCUMENT_NO = "DEMO-20261014-001";
const BUSINESS_DATE = "2026-10-14";
const CREATED_AT = "2026-10-14T01:20:00Z";
const POSTED_AT = "2026-10-14T02:30:00Z";
const OPERATOR = "演示操作人";
const PARTNER = "示例往来单位";
const REMARK = "首批交付，余量随第二批补齐。\n到货前请提前一天联系仓库。";
const DELIVERY = {
  contact: "演示联系人",
  phone: "13800000000",
  address: "演示市演示区演示路 1 号 3 号库",
};

const models = ["标准型", "紧凑型", "增强型"];
const specifications = ["600×400mm", "900×600mm", "1200×800mm"];
const units = ["件", "箱", "套"];

function lineAmount(quantity: number, unitPrice: number) {
  return ((quantity * Math.round(unitPrice * 100)) / 100).toFixed(2);
}

const shortItems: DocumentItem[] = [
  {
    productCode: "DEMO-001",
    productName: "演示商品 01",
    productModel: "标准型",
    productSpecification: "600×400mm",
    productType: "GOODS",
    quantity: "12",
    unit: "件",
    unitPrice: "38.00",
    amount: "456.00",
    remark: "首批交付。\n余量随第二批补齐。",
  },
  {
    productCode: "DEMO-002",
    productName: "演示商品 02",
    productModel: "紧凑型",
    productSpecification: "900×600mm",
    productType: "GOODS",
    quantity: "4",
    unit: "箱",
    unitPrice: "126.50",
    amount: "506.00",
  },
  {
    productCode: "DEMO-003",
    productName: "演示商品 03 · 用于检查超长商品名称不会撑破弹窗宽度",
    productModel: "增强型",
    productSpecification: "1200×800mm 加强型",
    productType: "GOODS",
    quantity: "3",
    unit: "套",
    unitPrice: "88.00",
    amount: "264.00",
  },
  {
    productCode: "DEMO-901",
    productName: "演示安装服务",
    productType: "SERVICE",
    quantity: "1",
    unit: "次",
    unitPrice: "200.00",
    amount: "200.00",
    remark: "服务不产生库存变动。",
  },
];

const longItems: DocumentItem[] = Array.from({ length: 24 }, (_, index) => {
  const sequence = index + 1;
  const quantity = (index % 5) + 1;
  const unitPrice = 100 + index * 5;

  return {
    productCode: `DEMO-${String(sequence).padStart(3, "0")}`,
    productName: `演示商品 ${String(sequence).padStart(2, "0")}`,
    productModel: models[index % models.length],
    productSpecification: specifications[index % specifications.length],
    productType: "GOODS" as const,
    quantity: String(quantity),
    unit: units[index % units.length],
    unitPrice: unitPrice.toFixed(2),
    amount: lineAmount(quantity, unitPrice),
  };
});

function itemsTotal(items: readonly DocumentItem[]) {
  const cents = items.reduce(
    (sum, item) => sum + Math.round(Number(item.amount ?? "0") * 100),
    0,
  );

  return (cents / 100).toFixed(2);
}

function wait(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

export function DocumentDialogDemoPage() {
  const [scale, setScale] = useState<"short" | "long">("short");
  const [detailOpen, setDetailOpen] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [posting, setPosting] = useState(false);
  const [postError, setPostError] = useState("");
  const [failSubmission, setFailSubmission] = useState(false);
  const [status, setStatus] = useState<"DRAFT" | "POSTED">("DRAFT");

  const items = scale === "short" ? shortItems : longItems;
  const total = itemsTotal(items);
  const statusText = status === "POSTED" ? "已过账" : "草稿";

  async function confirmPost() {
    setPosting(true);
    setPostError("");
    await wait(700);
    setPosting(false);

    if (failSubmission) {
      setPostError("演示：库存不足，整单被拒绝。");
      return;
    }

    setConfirmOpen(false);
    setStatus("POSTED");
    toast.success({
      title: "已模拟过账演示单据",
      description: "示例页只更新页面内存数据，不请求后端接口。",
    });
  }

  function resetDemo() {
    setStatus("DRAFT");
    setPostError("");
    setDetailOpen(false);
    setConfirmOpen(false);
    toast.info("演示状态已重置为草稿。");
  }

  const delivery = (
    <dl className="grid gap-space-3 text-sm sm:grid-cols-3">
      <div>
        <dt className="text-text-tertiary">联系人</dt>
        <dd>{DELIVERY.contact}</dd>
      </div>
      <div>
        <dt className="text-text-tertiary">电话</dt>
        <dd>{DELIVERY.phone}</dd>
      </div>
      <div>
        <dt className="text-text-tertiary">地址</dt>
        <dd className="break-words">{DELIVERY.address}</dd>
      </div>
    </dl>
  );

  return (
    <>
      <PageHeader
        title="单据弹窗 Demo"
        description="单据详情与过账确认示例，展示固定操作区、只读明细与折叠辅助信息。"
      />

      <div className="max-w-[1080px] space-y-6">
        <ContentCard
          title="演示单据"
          description="同一份数据对应两种阅读模式：详情表达“已发生”，确认表达“将发生”。"
        >
          <div className="space-y-space-5">
            <div className="grid gap-space-4 md:grid-cols-2">
              <Field
                label="数据规模"
                htmlFor="demo-scale"
                help="长单用于检查正文真实滚动时，头部与底部操作区保持不动。"
              >
                <Select
                  id="demo-scale"
                  value={scale}
                  onChange={(event) =>
                    setScale(event.target.value as "short" | "long")
                  }
                >
                  <option value="short">短单（4 行）</option>
                  <option value="long">长单（24 行）</option>
                </Select>
              </Field>

              <Field
                label="模拟提交失败"
                help="开启后确认过账会返回失败，用于检查错误提示与确认锁定。"
              >
                <div className="flex items-center gap-space-3">
                  <Switch
                    checked={failSubmission}
                    onCheckedChange={setFailSubmission}
                    aria-label="模拟提交失败"
                  />
                  <span className="text-sm text-text-secondary">
                    {failSubmission ? "已开启" : "已关闭"}
                  </span>
                </div>
              </Field>
            </div>

            <div className="flex flex-wrap items-center gap-space-3">
              <Button variant="primary" onClick={() => setDetailOpen(true)}>
                查看单据详情
              </Button>
              <Button
                variant="secondary"
                onClick={() => {
                  setPostError("");
                  setConfirmOpen(true);
                }}
              >
                保存并过账
              </Button>
              <Button variant="ghost" onClick={resetDemo}>
                重置演示状态
              </Button>
              <span className="flex items-center gap-space-2 text-sm text-text-tertiary">
                当前状态：
                <StatusTag tone={status === "POSTED" ? "success" : "neutral"}>
                  {statusText}
                </StatusTag>
              </span>
            </div>

            <p className="text-sm text-text-tertiary">
              本页只使用页面内存数据，不请求后端接口。
            </p>
          </div>
        </ContentCard>
      </div>

      <DetailDialog
        documentLayout
        fixedFooter
        open={detailOpen}
        title={`演示单据 · ${DOCUMENT_NO}`}
        description={`${PARTNER} · 业务日期 ${BUSINESS_DATE} · ${statusText}`}
        onCancel={() => setDetailOpen(false)}
        footer={
          <div className="flex flex-wrap items-center justify-between gap-space-2">
            <span className="text-sm text-text-tertiary">
              固定操作区：正文滚动时按钮保持可见。
            </span>
            <Button variant="secondary" onClick={() => setDetailOpen(false)}>
              关闭
            </Button>
          </div>
        }
      >
        <div className="space-y-space-5">
          <DocumentSummary
            totals={[
              { label: "单据金额", value: `¥${total}` },
              { label: "明细行数", value: `${items.length} 行` },
            ]}
            hint={
              <span className="font-medium">
                {status === "POSTED"
                  ? "已过账 · 库存与往来已生效"
                  : "草稿 · 尚未生效"}
              </span>
            }
          >
            <p>
              {status === "POSTED"
                ? "演示影响：实物商品已增加库存，供应商方向往来余额已增加。"
                : "草稿未改变库存或往来余额；过账后才生效。"}
            </p>
          </DocumentSummary>

          <DocumentItems items={items} />

          <section className="space-y-space-2">
            <h3 className="text-base font-semibold">单据备注</h3>
            <p className="whitespace-pre-wrap break-words text-sm">{REMARK}</p>
          </section>

          <DocumentDisclosure title="送货资料" hint="联系人、电话与地址">
            {delivery}
          </DocumentDisclosure>

          <DocumentDisclosure title="关联单据" hint="1 张来源单">
            <p className="text-sm text-text-secondary">
              来源单 SO-DEMO-0001 · 已过账。示例页不提供跳转。
            </p>
          </DocumentDisclosure>

          <DocumentOperationRecords
            document={{
              createdByName: OPERATOR,
              createTime: CREATED_AT,
              postedByName: status === "POSTED" ? OPERATOR : undefined,
              postedAt: status === "POSTED" ? POSTED_AT : null,
            }}
          />
        </div>
      </DetailDialog>

      <FormDialog
        documentLayout
        open={confirmOpen}
        title="确认过账演示单据"
        description={`${DOCUMENT_NO} · ${PARTNER} · 业务日期 ${BUSINESS_DATE}`}
        submitText={postError ? "请刷新后重新核对" : "确认过账"}
        cancelText={postError ? "关闭确认" : "取消"}
        loadingText="正在过账…"
        loading={posting}
        submitDisabled={Boolean(postError)}
        onCancel={() => {
          setConfirmOpen(false);
          setPostError("");
        }}
        onSubmit={confirmPost}
      >
        <div className="space-y-space-5">
          {postError && (
            <div
              role="alert"
              className="space-y-space-2 rounded-control border border-error/30 bg-error/5 p-space-3 text-sm text-error"
            >
              <p>过账失败：{postError}</p>
              <p>此确认已锁定。请根据上方核实结果处理，避免重复提交。</p>
            </div>
          )}

          <DocumentSummary
            totals={[
              { label: "将提交金额", value: `¥${total}` },
              { label: "明细行数", value: `${items.length} 行` },
            ]}
            hint="即将执行：过账演示单据"
          >
            <p>
              实物商品将增加库存；有金额时供应商方向往来余额增加。服务行不产生库存变动。
            </p>
          </DocumentSummary>

          <DocumentItems items={items} title="即将提交的明细" />

          <DocumentDisclosure title="送货资料" hint="补充资料，可折叠">
            {delivery}
          </DocumentDisclosure>
        </div>
      </FormDialog>
    </>
  );
}
