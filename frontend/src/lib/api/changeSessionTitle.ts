import toast from "react-hot-toast";
import {UpdateSessionTitleRequest} from "@/lib/Interface";
import client from "@/utils/client";
import type { AxiosError } from "axios";

export const updateSessionTitle = async (sessionId: number, newTitle: string) => {
    try {
        const requestData: UpdateSessionTitleRequest = {
            new_title: newTitle
        };

        const response = await client.post(`/chat_session/${sessionId}/title`, requestData);
        if (response.data && response.data.data) {
            return response.data.data;
        }
        return response.data;
    } catch (err) {
        const axiosErr = err as AxiosError<{ message?: string; msg?: string }>; 
        const errorMessage = axiosErr.response?.data?.message || axiosErr.response?.data?.msg || (axiosErr as Error).message || '修改会话标题失败';
        toast.error(errorMessage);
        throw err;
    }
};
