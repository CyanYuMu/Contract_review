import client, {getAuthToken} from '@/utils/client';
import type { AxiosError } from 'axios';

// 启动比对任务
export const startComparisonTask = async (standardFileId: number, comparisonFileId: number, title?: string, sessionId?: number) => {
    try {
        const token = getAuthToken();
        const response = await client.post('/comparison_task/start', {
            standard_file_id: standardFileId,
            comparison_file_id: comparisonFileId,
            title: title,
            session_id: sessionId
        }, {
            headers: {
                ...(token && {'Authorization': `Bearer ${token}`}),
            },
        });
        return response.data;
    } catch (error) {
        const axiosErr = error as AxiosError<{ message?: string; msg?: string; error?: string; details?: string }>;
        if (axiosErr.response) {
            const message =
                axiosErr.response.data?.message ||
                axiosErr.response.data?.msg ||
                axiosErr.response.data?.error ||
                axiosErr.response.data?.details ||
                `比对任务启动失败（${axiosErr.response.status}）`;
            throw new Error(message);
        } else if (axiosErr.request) {
            throw new Error('网络连接失败，请检查网络');
        } else {
            throw new Error((error as Error).message || '比对任务启动失败');
        }
    }
};
