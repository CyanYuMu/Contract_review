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

export interface ReviewProgressEvent {
    phase: string;
    agent: string;
    status: "running" | "completed" | "failed" | string;
    message: string;
    progress: number;
    timestamp: string;
    data?: unknown;
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
    type?: string;
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

// ============ 合同问答 QA ============
export interface QAMessage {
    id: number;
    session_id: number;
    role: "user" | "assistant";
    content: string;
    tokens: number;
    created_at: string;
}

export interface QAAskRequest {
    session_id: number;
    message: string;
}

export interface QAAskEndData {
    message_id: number;
    tokens: number;
    cache_hit: boolean;
}

export interface QAAskCallbacks {
    onDelta?: (delta: string) => void;
    onEnd?: (data: QAAskEndData) => void;
    onError?: (error: Error) => void;
}

// 合同列表项（新建问答时绑定合同用；Contract 结构无 json tag，字段为大写）
export interface ContractListItem {
    ID: number;
    Title: string;
    FilePath: string;
    FileType: string;
    PartyA: string;
    PartyB: string;
    Amount: number;
    ContractType?: { id: number; name: string } | null;
}

// ============ 大模型网关 成本 / 路由 / 配额 ============
export interface UsageStat {
    feature: string;
    model_name: string;
    call_count: number;
    prompt_tokens: number;
    completion_tokens: number;
    total_tokens: number;
    cost: number;
    cache_hit_count: number;
}

export interface DailyUsageTrend {
    date: string;
    total_tokens: number;
    cost: number;
    call_count: number;
}

export interface GatewayRoute {
    feature: string;
    model_config_id: number;
    model_name: string;
    params: string;
    updated_at: string;
}

export interface LLMQuota {
    id?: number;
    subject_type: string;
    subject_id: number;
    feature?: string;
    daily_token_limit: number;
    monthly_token_limit: number;
    updated_at?: string;
}
