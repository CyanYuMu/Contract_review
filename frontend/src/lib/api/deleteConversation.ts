import toast from "react-hot-toast";
import client from "@/utils/client";
import type { AxiosError } from "axios";

export const deleteConversation = async (userId: string, sessionId: string) => {
    try {
        const response = await client.delete(`/session/${userId}/${sessionId}`);
        return response.data;
    } catch (err) {
        const axiosErr = err as AxiosError<{ message?: string; msg?: string }>; 
        const errorMessage = axiosErr.response?.data?.message || axiosErr.response?.data?.msg || (axiosErr as Error).message || '删除对话失败';
        toast.error(errorMessage);
        throw err;
    }
}