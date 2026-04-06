import client from '@/utils/client';
import type {LoginRequest} from '@/lib/Interface';
import type {AxiosError} from 'axios';

export const login = async (data: LoginRequest) => {
    try {
        const response = await client.post('/user/login', data);
        return response;
    } catch (error) {
        const axiosErr = error as AxiosError<{ message?: string; msg?: string }>;
        if (axiosErr.response) {
            if (axiosErr.response.status === 401) {
                throw new Error('账号或密码错误');
            }
        } else if (axiosErr.request) {
            throw new Error('网络连接失败，请检查网络');
        } else {
            throw new Error((error as Error).message || '登录失败');
        }
    }
};
