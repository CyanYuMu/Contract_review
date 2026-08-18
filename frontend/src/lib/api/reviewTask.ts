import client from '@/utils/client';

export type ReviewTaskStatus = 'pending' | 'processing' | 'completed' | 'failed';

export interface ReviewTaskInfo {
    id?: number;
    session_id?: number;
    file_id?: number;
    stance?: string;
    intensity?: string;
    contract_type?: string;
    status?: ReviewTaskStatus;
    created_at?: string;
    completed_at?: string;
}

/**
 * 获取审阅任务状态 GET /review/task?session_id=
 * 后端返回 {code,msg,data:{...}}，取 data。
 */
export const getReviewTaskStatus = async (sessionId: number): Promise<ReviewTaskInfo | null> => {
    try {
        const response = await client.get('/review/task', {params: {session_id: sessionId}});
        return response.data?.data ?? null;
    } catch {
        // 404 = 任务不存在（可能从未启动），返回 null 让调用方跳过
        return null;
    }
};

/**
 * 获取审阅结果（按会话） GET /review/results/session?session_id=
 * 返回 {code,msg,data:{list,total}}，取 data.list。
 */
export const getReviewResults = async (sessionId: number): Promise<unknown[]> => {
    try {
        const response = await client.get('/review/results/session', {params: {session_id: sessionId}});
        return response.data?.data?.list ?? [];
    } catch {
        return [];
    }
};
