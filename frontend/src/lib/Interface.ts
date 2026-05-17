export type User = {
    id?: number;
    userId?: number;
    account?: string;
    username: string;
    is_active?: boolean;
    permissions?: Record<string, boolean>;
};
export type ApiError = Error & { status?: number };
export interface SessionListParams {
    page?: number;
    size?: number;
}

export interface UpdateSessionTitleRequest {
    new_title: string;
}

export interface CreateConversationRequest {
    title: string;
    file_id?: number | null;
    session_type: string;
}

export interface StartTaskRequest {
    session_id: number;
    stance: string;
    contract_type: string;
    intensity: string;
    description?: string | null;
}

export interface ReviewTaskResponse {
    id: number;
    session_id: number;
    contract_id: number;
    user_id: number;
    stance: string;
    intensity: string;
    description: string | null;
    status: string;
    created_at: string;
    completed_at: string | null;
}

export interface GenericResponse<T> {
    code: number | null;
    msg: string | null;
    data: T | null;
}

export interface LoginRequest {
    account: string;
    password: string;
}

export interface RegisterRequest {
    account: string;
    username: string;
    password: string;
}


export interface RefreshTokenRequest {
    refresh_token: string;
}


export interface TokenResponse {
    access_token: string;
    token_type: string;
    refresh_token: string;
    expires_in?: number;
}

export interface GetUserConversationRequest {
    user_id: string;
}

export interface CreateConversationWithSessionRequest {
    title: string;
    contract_id: number;
    session_id: string;
    session_type?: string;
}

// ============ SSE 聊天相关类型 ============
/**
 * SSE 聊天请求参数（基于后端API）
 */
export interface ChatRequest {
    message: string;
    user_id: string;
    session_id: string;
    action: string; // 例如: "chat"
    role?: string | null;
    contract_type?: string | null;
}

/**
 * SSE 聊天响应（后端返回的完整响应）
 */
export interface ChatResponse {
    response: string;
}

/**
 * 聊天消息类型（用于前端消息列表）
 */
export interface ChatMessage {
    id: string;
    role: "user" | "assistant";
    content: string;
    timestamp: number;
}

export interface SendMessageRequest {
    session_id?: number | null; // 可选：不传则由后端按策略创建/处理
    role?: "user" | "assistant" | "system";
    content: string;
    parent_id?: number | null;
}

export interface SendMessageResponse {
    message_id: number;
    session_id: number;
    role: string;
    content: string;
    created_at: string;
    parent_id?: number | null;
}

// 获取会话消息列表相关接口
export interface MessageListItem {
    message_id: number;
    session_id: number;
    role: string;
    content: string;
    created_at: string;
    parent_id?: number | null;
}

export interface GetMessageListParams {
    session_id: number;
    page?: number;
    size?: number;
}

export interface MessageListResponse {
    total: number;
    messages: MessageListItem[];
}

//sse风险点
export interface RiskResponse {
    id: number;
    session_id: number;
    task_id: number;
    index: number;
    original_content: string;
    risk_level: string;
    risk_type?: string;
    risk_analysis: string;
    suggested_content: string;
    reason?: string;
    is_accepted: boolean;
    created_at: string;
}

export interface ReplaceProps {
    original_content: string;
    suggested_content: string;
}

export interface OverviewResponse {
    reviewed_contracts?: number;
    service_departments: number;
    served_faculty_students: number;
    total_reviewed_amount: number;
}

export interface RevisionsResponse {
    risk_points_revised: number;
    error_points_revised: number;
}

export interface ContractTypesResponse {
    reviewed_contracts: number;
    using_units: string[];
    using_count: number;
    service_contracts: number;
    goods_contracts: number;
    infrastructure_contracts: number;
}

export interface TrendItem {
    date?: string | null;
    total: number;
    service: number;
    goods: number;
    infrastructure: number;
}

export interface TrendParams {
    period?: string;
    start_date?: string | null;
    end_date?: string | null;
    contract_type_ids?: number[] | null;
}

export interface DepartmentUsageItem {
    department_name: string;
    contract_review: number;
    contract_verification: number;
    contract_comparison: number;
    total: number;
}

export interface WordCloudItem {
    name: string;
    value: number;
}

//智审记录
export interface historyType {
    id?: number;
    title: string;
    session_type: string;
    is_accepted?: boolean;
    created_at: string;
    partyA: string;
    partyB: string;
    file_path?: string;
    file_id?: number;
}

export interface contrastType {
    id?: number;
    origin_contract_name: string;
    new_contract_name: string;
    similarity: number;
    status: boolean;
    dateRange: string;
    file_path?: string;
    file_id?: number;
    file_path_2?: string;
    file_id_2?: number;
    original_file_path?: string;
    comparison_file_path?: string;
    original_file_id?: number;
    comparison_file_id?: number;
    standard_download_url?: string;
    comparison_download_url?: string;
    download_url?: string;
    download_url_2?: string;
}

export interface ModelsListParams {
    page?: number;
    size?: number;
}

export interface PromptListParams {
    contract_type_id: number;
    organization_id: number;
}

//智审记录
export interface ListSessionResponse {
    id: number;
    title: string;
    session_type: string;
    file_id: number;
    created_at: string;
}
