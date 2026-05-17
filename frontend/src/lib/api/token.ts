import axios from 'axios';
import type { TokenResponse } from '@/lib/Interface';
import type { AxiosError } from 'axios';


export const refreshToken = async (refreshToken: string): Promise<TokenResponse> => {
    try {
        const response = await axios.post('/user/refresh_token', {
        // const response = await axios.post('/user/refresh_token', {
            refresh_token: refreshToken
        }, {
            // baseURL: '/api',
            baseURL: '/api/proxy',// 上线时使用该配置
            headers: {
                'Content-Type': 'application/json',
            },
        });
        
        // 检查响应格式
        if (response.data?.code !== 200 || !response.data?.data?.access_token) {
            throw new Error('刷新token失败，响应格式错误');
        }

        // 适配后端返回格式，补充默认值
        return {
            access_token: response.data.data.access_token,
            token_type: 'Bearer',
            refresh_token: refreshToken, // 后端刷新接口不返回新的refresh_token，继续使用旧的
            expires_in: response.data.data.expires_in
        };
    } catch (error) {
        const axiosErr = error as AxiosError<{ message?: string; msg?: string }>;
        
        if (axiosErr.response) {
            if (axiosErr.response.status === 401) {
              
                throw new Error('刷新token失败，refresh_token已过期');
            }
            throw new Error(axiosErr.response.data?.message || axiosErr.response.data?.msg || '刷新token失败');
        } else if (axiosErr.request) {
            throw new Error('网络连接失败，请检查网络');
        } else {
            throw new Error((error as Error).message || '刷新token失败');
        }
    }
};
