import client, {getAuthToken, clearTokenInfo} from '@/utils/client';
import {authDatedHandler} from '@/utils/authDatedHandler';
import type {AxiosError} from 'axios';
import type {QAAskRequest, QAAskCallbacks, QAMessage, ContractListItem} from '@/lib/Interface';

/**
 * 合同问答 SSE 流式提问
 * POST /api/proxy/qa/ask  {session_id, message}
 * 后端 SSE 事件：delta{content} / error{message} / end{message_id,tokens,cache_hit}
 */
export const askQuestion = async (req: QAAskRequest, callbacks: QAAskCallbacks): Promise<void> => {
    const token = getAuthToken();

    const response = await fetch('/api/proxy/qa/ask', {
        method: 'POST',
        body: JSON.stringify(req),
        headers: {
            'Content-Type': 'application/json',
            ...(token && {Authorization: `Bearer ${token}`}),
            Accept: 'text/event-stream',
        },
    });

    if (!response.ok) {
        if (response.status === 401 || response.status === 403) {
            clearTokenInfo();
            authDatedHandler.trigger403Error();
        }
        const text = await response.text().catch(() => '');
        const error = new Error(text || `请求失败: ${response.status} ${response.statusText}`);
        callbacks.onError?.(error);
        throw error;
    }

    if (!response.body) {
        callbacks.onError?.(new Error('无响应流'));
        return;
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    let ended = false;

    const handleEvent = (eventText: string) => {
        const dataLines = eventText
            .split(/\r?\n/)
            .map((l) => l.trim())
            .filter((l) => l && !l.startsWith(':') && l.startsWith('data:'))
            .map((l) => l.slice(5).trim());

        for (const line of dataLines) {
            if (!line) continue;
            try {
                const parsed = JSON.parse(line);
                if (parsed.event === 'delta' && parsed.data?.content) {
                    callbacks.onDelta?.(parsed.data.content as string);
                } else if (parsed.event === 'error') {
                    const msg = parsed.data?.message || '问答失败';
                    callbacks.onError?.(new Error(msg));
                    ended = true;
                    return;
                } else if (parsed.event === 'end') {
                    callbacks.onEnd?.({
                        message_id: parsed.data?.message_id ?? 0,
                        tokens: parsed.data?.tokens ?? 0,
                        cache_hit: Boolean(parsed.data?.cache_hit),
                    });
                    ended = true;
                    return;
                }
            } catch {
                // 忽略无法解析的行
            }
        }
    };

    try {
        while (true) {
            const {done, value} = await reader.read();
            if (done) {
                const remaining = buffer + decoder.decode();
                if (remaining.trim()) handleEvent(remaining);
                break;
            }
            buffer += decoder.decode(value, {stream: true});
            const events = buffer.split(/\r?\n\r?\n/);
            buffer = events.pop() || '';
            for (const evt of events) {
                handleEvent(evt);
                if (ended) {
                    await reader.cancel().catch(() => undefined);
                    return;
                }
            }
        }
    } catch (error) {
        if (!ended) callbacks.onError?.(error as Error);
        throw error;
    }
};

/** 获取会话问答历史 GET /qa/messages?session_id=&limit= */
export const getQAMessages = async (sessionId: number, limit = 50): Promise<QAMessage[]> => {
    try {
        const response = await client.get('/qa/messages', {params: {session_id: sessionId, limit}});
        return response.data?.data || [];
    } catch (error) {
        const axiosErr = error as AxiosError<{msg?: string}>;
        throw new Error(axiosErr.response?.data?.msg || '获取问答历史失败');
    }
};

/** 清空会话问答历史 POST /qa/clear {session_id} */
export const clearQAMessages = async (sessionId: number): Promise<void> => {
    try {
        await client.post('/qa/clear', {session_id: sessionId});
    } catch (error) {
        const axiosErr = error as AxiosError<{msg?: string}>;
        throw new Error(axiosErr.response?.data?.msg || '清空问答历史失败');
    }
};

/** 获取合同列表（新建问答时绑定合同）GET /contract/list?page=&page_size= */
export const getContractList = async (
    page = 1,
    pageSize = 50
): Promise<{list: ContractListItem[]; total: number}> => {
    try {
        const response = await client.get('/contract/list', {params: {page, page_size: pageSize}});
        return response.data?.data || {list: [], total: 0};
    } catch (error) {
        const axiosErr = error as AxiosError<{msg?: string}>;
        throw new Error(axiosErr.response?.data?.msg || '获取合同列表失败');
    }
};
