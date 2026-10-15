import { http } from "@/lib/http";
import type { ApiPageResult } from "@/types/api";
import type {
  McpTokenCreateRequest,
  McpTokenExtendRequest,
  McpTokenListQuery,
  McpTokenRecord,
  McpTokenSecret,
  McpTokenStatusRequest,
} from "@/types/mcp-token";

const MCP_TOKEN_BASE_PATH = "/api/system/mcp/tokens";

export function getMcpTokenPage(query: McpTokenListQuery) {
  return http.get<ApiPageResult<McpTokenRecord>>(`${MCP_TOKEN_BASE_PATH}/page`, {
    query,
  });
}

/** 创建令牌；响应带原值，必须立即展示并复制。 */
export function createMcpToken(data: McpTokenCreateRequest) {
  return http.post<McpTokenSecret>(MCP_TOKEN_BASE_PATH, data);
}

/** 查看原值（独立接口，不混入列表）。 */
export function getMcpTokenSecret(id: number) {
  return http.get<McpTokenSecret>(`${MCP_TOKEN_BASE_PATH}/${id}/secret`);
}

export function extendMcpToken(id: number, data: McpTokenExtendRequest) {
  return http.post<McpTokenRecord>(`${MCP_TOKEN_BASE_PATH}/${id}/extend`, data);
}

export function updateMcpTokenStatus(id: number, data: McpTokenStatusRequest) {
  return http.put<McpTokenRecord>(`${MCP_TOKEN_BASE_PATH}/${id}/status`, data);
}

/** 重置令牌；旧值立即失效，响应带新原值。 */
export function resetMcpToken(id: number) {
  return http.post<McpTokenSecret>(`${MCP_TOKEN_BASE_PATH}/${id}/reset`);
}
