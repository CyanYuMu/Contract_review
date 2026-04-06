import client, {getAuthToken} from '@/utils/client';
import type {AxiosError} from 'axios';
import {CreateConversationRequest} from "@/lib/Interface";

export const createSession = async (data: CreateConversationRequest) => {
    try {
        const token = getAuthToken();
        const response = await client.post('/session/create_session', data, {
            headers: {
                'Content-Type': 'application/json',
                ...(token && {'Authorization': `Bearer ${token}`}),
            },
        });  
        return response.data.data || response.data;
    } catch (error) {
        const axiosErr = error as AxiosError<{ message?: string; msg?: string }>;
        const message = axiosErr.response?.data?.message || axiosErr.response?.data?.msg || (axiosErr as Error).message || '创建会话失败';
        throw new Error(message);
    }
};