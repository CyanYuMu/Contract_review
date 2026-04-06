import toast from 'react-hot-toast'
import {ChatRequest, ChatResponse} from '@/lib/Interface'
import client from '@/utils/client'
import type { AxiosError } from 'axios'

export const chat = async (data: ChatRequest): Promise<ChatResponse> => {
    try {
        const response = await client.post('/chat', data);
        return response.data;
    } catch (err) {
        const axiosErr = err as AxiosError<{ message?: string; msg?: string }>; 
        const errorMessage = axiosErr.response?.data?.message || axiosErr.response?.data?.msg || (axiosErr as Error).message || '聊天请求失败';
        toast.error(errorMessage);
        throw err;
    }
}

