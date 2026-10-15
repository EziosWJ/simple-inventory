import type { ApiPageRequest } from "./api";

/** 后端筛选状态枚举；空字符串代表“全部”。 */
export type McpTokenState = "ENABLED" | "DISABLED" | "EXPIRED";

export type McpTokenRecord = {
  id: number;
  /** 令牌名称 */
  name: string;
  userId: number;
  /** 绑定用户昵称 */
  userName: string;
  /** 是否启用（含未吊销） */
  enabled: boolean;
  /** 是否永久有效 */
  permanent: boolean;
  /** RFC3339 UTC；permanent 时为 null */
  expiresAt: string | null;
  /** 是否已过期 */
  expired: boolean;
  /** 旧版吊销记录 */
  revoked: boolean;
  /** enabled && !expired */
  usable: boolean;
  /** 是否支持查看原值 */
  secretViewable: boolean;
  /** 旧版仅摘要令牌，重置后才可查看 */
  legacy: boolean;
  /** RFC3339 UTC */
  createdAt: string;
  createdBy: string;
};

export type McpTokenListQuery = Partial<ApiPageRequest> & {
  /** 按名称模糊 */
  keyword?: string;
  userId?: number;
  state?: McpTokenState;
};

/**
 * 创建请求三选一（互斥）：永久 / 指定天数 / 指定日历日。
 * expiresOn 为 Asia/Shanghai 日历日（含当天），按 YYYY-MM-DD 原样发送。
 */
export type McpTokenCreateRequest =
  | { name: string; userId: number; permanent: true }
  | { name: string; userId: number; expiresInDays: number }
  | { name: string; userId: number; expiresOn: string };

export type McpTokenExtendRequest =
  | { days: number }
  | { permanent: true };

export type McpTokenStatusRequest = {
  enabled: boolean;
};

/** 查看原值、创建、重置接口的统一返回体。 */
export type McpTokenSecret = {
  token: string;
  permanent: boolean;
  expiresAt: string | null;
  record: McpTokenRecord;
};
