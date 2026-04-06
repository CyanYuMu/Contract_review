import toast from "react-hot-toast";
import client, { getAuthToken } from "@/utils/client";
import type { AxiosError } from "axios";

/**
 * 更新模型配置
 * @param modelId 模型ID
 * @param data 更新数据
 */
export const updateModel = async (modelId: string | number, data: Record<string, any>) => {
    try {
        const token = getAuthToken();
        const response = await client.put(`/model_configs/update_model/${modelId}`, data, {
            headers: {
                'Content-Type': 'application/json',
                ...(token && { 'Authorization': `Bearer ${token}` }),
            },
        });
        if (response.data && response.data.data) {
            return response.data.data;
        }
        return response.data;
    } catch (err) {
        const axiosErr = err as AxiosError<{ message?: string; msg?: string }>;
        const errorMessage = axiosErr.response?.data?.message || axiosErr.response?.data?.msg || (axiosErr as Error).message || '更新模型配置失败';
        toast.error(errorMessage);
        throw err;
    }
};

/**
 * 创建模型配置
 * @param data 创建数据
 */
export const createModel = async (data: Record<string, any>) => {
    try {
        const token = getAuthToken();
        const response = await client.post('/model_configs/create_model', data, {
            headers: {
                'Content-Type': 'application/json',
                ...(token && { 'Authorization': `Bearer ${token}` }),
            },
        });
        if (response.data && response.data.data) {
            return response.data.data;
        }
        return response.data;
    } catch (err) {
        const axiosErr = err as AxiosError<{ message?: string; msg?: string }>;
        const errorMessage = axiosErr.response?.data?.message || axiosErr.response?.data?.msg || (axiosErr as Error).message || '创建模型配置失败';
        toast.error(errorMessage);
        throw err;
    }
};

/**
 * 获取模型配置
 */
export const getDefaultModel = async () => {
    try {
        const token = getAuthToken();
        const response = await client.get('/model_configs/get_default_model', {
            headers: {
                'Content-Type': 'application/json',
                ...(token && { 'Authorization': `Bearer ${token}` }),
            },
        });
        if (response.data && response.data.data) {
            return response.data.data;
        }
        return response.data;
    } catch (err) {
        const axiosErr = err as AxiosError<{ message?: string; msg?: string }>;
        const errorMessage = axiosErr.response?.data?.message || axiosErr.response?.data?.msg || (axiosErr as Error).message || '获取模型配置失败';
        toast.error(errorMessage);
        throw err;
    }
};
