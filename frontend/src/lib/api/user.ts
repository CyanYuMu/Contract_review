import client from '@/utils/client';
import type { AxiosError } from 'axios';
import { getCachedUser, setCachedUser } from '@/utils/userCache';

/**
 * 获取用户信息
 * 优先从缓存获取，如果没有缓存或强制刷新则请求接口
 * @param forceRefresh 是否强制刷新（忽略缓存）
 */
export const getUserInfo = async (forceRefresh = false) => {
    // 如果不是强制刷新，先检查缓存
    if (!forceRefresh) {
        const cachedUser = getCachedUser();
        if (cachedUser) {
            return cachedUser;
        }
    }

    try {
        const response = await client.get('/user/me');

        let userData;
        if (response.data && response.data.data) {
            userData = response.data.data;
        } else {
            userData = response.data;
        }

        // 缓存用户信息
        if (userData) {
            setCachedUser(userData);
        }

        return userData;
    } catch (error) {
        const axiosErr = error as AxiosError<{ message?: string; msg?: string }>; 
        if (axiosErr.response) {
            const message = axiosErr.response.data?.message || axiosErr.response.data?.msg || `获取用户信息失败（${axiosErr.response.status}）`;
            const err = new Error(message);
            (err as Error & { status?: number }).status = axiosErr.response.status;
            throw err;
        } else if (axiosErr.request) {
            throw new Error('网络连接失败，请检查网络');
        } else {
            throw new Error((error as Error).message || '获取用户信息失败');
        }
    }
};
