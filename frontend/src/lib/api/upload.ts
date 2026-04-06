import client from '@/utils/client';
import type { AxiosError } from 'axios';

export const upload = async (data: FormData, onProgress?: (progress: number) => void) => {
    try {
        const response = await client.post('/contract/upload', data, {
            headers:{
              "Content-Type": "multipart/form-data",
            },
            onUploadProgress: (progressEvent) => {
                if (progressEvent.total && onProgress) {
                    const percentCompleted = Math.round((progressEvent.loaded * 100) / progressEvent.total);
                    onProgress(percentCompleted);
                }
            },
        });
        return response.data;
    } catch (error) {
        const axiosErr = error as AxiosError<{ message?: string }>; 
        if (axiosErr.response) {
            const message = axiosErr.response.data?.message || `上传失败,请登录（${axiosErr.response.status}）`;
            throw new Error(message);
        } else if (axiosErr.request) {
            throw new Error('网络连接失败，请检查网络');
        } else {
            throw new Error((error as Error).message || '上传失败');
        }
    }
};


export const save = async (data: FormData) => {
    try {
        const response = await client.post('/contract/save_file', data);
        return response.data;
    } catch (error) {
        const axiosErr = error as AxiosError<{ message?: string }>;
        if (axiosErr.response) {
            const message = axiosErr.response.data?.message || `上传失败,请登录（${axiosErr.response.status}）`;
            throw new Error(message);
        } else if (axiosErr.request) {
            throw new Error('网络连接失败，请检查网络');
        } else {
            throw new Error((error as Error).message || '上传失败');
        }
    }
};


