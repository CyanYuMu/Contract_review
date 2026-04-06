import client from "@/utils/client";

export type ListSessionRequest = {
    page: number;
    page_size: number;
    session_type: string;
};
export const getListSession = async (data: ListSessionRequest) => {
    const response = await client.post('/session/list_sessions', data)
    return response.data;
}