import toast from "react-hot-toast";
import client from "@/utils/client";
import type { CreateConversationWithSessionRequest } from "@/lib/Interface";
import type { AxiosError } from "axios";

export const createConversation = async (data: CreateConversationWithSessionRequest) => {
    try {
        const response = await client.post('/chat_session/create', data);
        return response.data;
    } catch (err) {
        const axiosErr = err as AxiosError<{ message?: string; msg?: string }>; 
        const errorMessage = axiosErr.response?.data?.message || axiosErr.response?.data?.msg || (axiosErr as Error).message || '创建对话失败';
        toast.error(errorMessage);
        throw err;
    }
}
