import client from "@/utils/client";
export const getHistoryDetail = async (session_id: number | string) => {
    // 后端 SessionHistoryDetailRequest.SessionID 为 uint64，
    // 传字符串会被 Hertz 拒绝并返回 400"请求参数错误"，这里统一转成数字。
    const response = await client.post('/session/session_history_detail', {
        session_id: Number(session_id),
    });
    return response.data;
}
