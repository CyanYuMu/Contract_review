import client from '@/utils/client';
import type {RegisterRequest} from '@/lib/Interface';
import type {AxiosError} from 'axios';

export const register = async (data: RegisterRequest) => {
    try {
        const response = await client.post('/user/create', data);
        return response.data;
    } catch (error) {
        const axiosErr = error as AxiosError<{ message?: string; msg?: string }>;
        if (axiosErr.response) {
            const status = axiosErr.response.status;
            if (status === 409) {
                throw new Error("用户名已存在！")
            }
            const message = axiosErr.response.data?.message || axiosErr.response.data?.msg || `注册失败（${axiosErr.response.status}）`;
            throw new Error(message);
        } else if (axiosErr.request) {
            throw new Error('网络连接失败，请检查网络');
        } else {
            throw new Error((error as Error).message || '注册失败');
        }
    }
};
