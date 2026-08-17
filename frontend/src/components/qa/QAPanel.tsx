'use client';

import React, {useEffect, useState, useRef, useCallback} from 'react';
import {Button, Input, Spin, Empty, Modal, Select, Tooltip, message} from 'antd';
import {PlusOutlined, DeleteOutlined, SendOutlined, ClearOutlined, StopOutlined} from '@ant-design/icons';
import {useQAStore} from '@/store/qaStore';
import {askQuestion, getQAMessages, clearQAMessages, getContractList} from '@/lib/api/qa';
import {createSession} from '@/lib/api/createSession';
import {getListSession} from '@/lib/api/getListSession';
import {deleteSession} from '@/lib/api/deleteSession';
import type {ContractListItem, ListSessionResponse} from '@/lib/Interface';

const {TextArea} = Input;

type RawSession = {
    id?: number;
    session_id?: number;
    title?: string;
    session_type?: string;
    file_id?: number;
    created_at?: string;
};

/** 合同问答主面板：左侧会话列表 + 右侧流式对话区 */
export default function QAPanel() {
    const {
        sessions, currentSessionId, messages, isStreaming, streamingContent,
        setSessions, setCurrentSession, setMessages, appendMessage,
        startStreaming, appendDelta, resetStreaming, clearMessages,
    } = useQAStore();

    const [input, setInput] = useState('');
    const [loadingSessions, setLoadingSessions] = useState(false);
    const [loadingMessages, setLoadingMessages] = useState(false);
    const [pickerOpen, setPickerOpen] = useState(false);
    const [contracts, setContracts] = useState<ContractListItem[]>([]);
    const [loadingContracts, setLoadingContracts] = useState(false);
    const [selectedFileId, setSelectedFileId] = useState<number | null>(null);
    const [creating, setCreating] = useState(false);
    const messagesEndRef = useRef<HTMLDivElement>(null);
    const activeRequestRef = useRef<{
        sessionId: number;
        generation: number;
        controller: AbortController;
    } | null>(null);
    const requestGenerationRef = useRef(0);
    const historyGenerationRef = useRef(0);

    // 加载问答会话列表
    const loadSessions = useCallback(async () => {
        setLoadingSessions(true);
        try {
            const res = await getListSession({page: 1, page_size: 100, session_type: 'chat'});
            const inner = res?.data ?? res;
            const rawList = inner?.data?.data ?? inner?.data ?? [];
            const list: ListSessionResponse[] = (Array.isArray(rawList) ? rawList : [])
                .map((item: RawSession) => ({
                    id: item.id ?? item.session_id ?? 0,
                    title: item.title || '未命名问答',
                    session_type: item.session_type || 'chat',
                    file_id: item.file_id ?? 0,
                    created_at: item.created_at || '',
                }))
                .filter((s) => s.id);
            setSessions(list);
        } catch {
            message.error('加载会话列表失败');
        } finally {
            setLoadingSessions(false);
        }
    }, [setSessions]);

    useEffect(() => {
        loadSessions();
    }, [loadSessions]);

    // 切换会话时加载历史消息
    const loadMessages = useCallback(async (sessionId: number) => {
        const generation = ++historyGenerationRef.current;
        setLoadingMessages(true);
        try {
            const msgs = await getQAMessages(sessionId, 50);
            if (
                generation !== historyGenerationRef.current ||
                useQAStore.getState().currentSessionId !== sessionId
            ) {
                return;
            }
            setMessages(msgs);
        } catch {
            if (
                generation !== historyGenerationRef.current ||
                useQAStore.getState().currentSessionId !== sessionId
            ) {
                return;
            }
            message.error('加载问答历史失败');
            setMessages([]);
        } finally {
            if (generation === historyGenerationRef.current) {
                setLoadingMessages(false);
            }
        }
    }, [setMessages]);

    useEffect(() => {
        activeRequestRef.current?.controller.abort();
        activeRequestRef.current = null;
        requestGenerationRef.current += 1;
        resetStreaming();
        if (currentSessionId) loadMessages(currentSessionId);
        else setMessages([]);
    }, [currentSessionId, loadMessages, resetStreaming, setMessages]);

    useEffect(() => {
        return () => {
            activeRequestRef.current?.controller.abort();
            activeRequestRef.current = null;
            requestGenerationRef.current += 1;
        };
    }, []);

    // 流式输出时自动滚动到底部
    useEffect(() => {
        messagesEndRef.current?.scrollIntoView({behavior: 'smooth'});
    }, [messages, streamingContent, isStreaming]);

    // 打开合同选择器
    const openPicker = async () => {
        setPickerOpen(true);
        setSelectedFileId(null);
        setLoadingContracts(true);
        try {
            const {list} = await getContractList(1, 100);
            setContracts(list || []);
        } catch {
            message.error('获取合同列表失败');
            setContracts([]);
        } finally {
            setLoadingContracts(false);
        }
    };

    // 确认新建问答会话（绑定合同）
    const handleCreate = async () => {
        if (!selectedFileId) {
            message.warning('请选择一份合同');
            return;
        }
        const contract = contracts.find((c) => c.ID === selectedFileId);
        const title = `${contract?.Title || '合同'} 问答`;
        setCreating(true);
        try {
            const res = await createSession({title, file_id: selectedFileId, session_type: 'chat'});
            const newId = res?.session_id ?? res?.id ?? res?.data?.session_id;
            await loadSessions();
            if (newId) setCurrentSession(Number(newId));
            setPickerOpen(false);
            message.success('已创建问答会话');
        } catch {
            // createSession 内部已抛错
        } finally {
            setCreating(false);
        }
    };

    // 删除会话
    const handleDelete = async (id: number) => {
        try {
            await deleteSession(String(id));
            if (currentSessionId === id) {
                setCurrentSession(null);
                setMessages([]);
            }
            await loadSessions();
            message.success('已删除');
        } catch {
            // deleteSession 内部已 toast
        }
    };

    // 发送消息（SSE 流式）
    const handleSend = async () => {
        const text = input.trim();
        if (!text || isStreaming || !currentSessionId) return;
        const sessionId = currentSessionId;
        const generation = ++requestGenerationRef.current;
        const controller = new AbortController();
        activeRequestRef.current = {sessionId, generation, controller};
        const isActiveRequest = () => {
            const active = activeRequestRef.current;
            return Boolean(
                active &&
                active.sessionId === sessionId &&
                active.generation === generation &&
                !controller.signal.aborted
            );
        };
        setInput('');

        // 乐观追加用户消息
        appendMessage({
            id: Date.now(),
            session_id: sessionId,
            role: 'user',
            content: text,
            tokens: 0,
            created_at: new Date().toISOString(),
        });
        startStreaming();

        try {
            await askQuestion(
                {session_id: sessionId, message: text},
                {
                    onDelta: (delta) => {
                        if (isActiveRequest()) appendDelta(delta);
                    },
                    onEnd: (data) => {
                        if (!isActiveRequest()) return;
                        appendMessage({
                            id: data.message_id || Date.now(),
                            session_id: sessionId,
                            role: 'assistant',
                            content: useQAStore.getState().streamingContent,
                            tokens: data.tokens,
                            created_at: new Date().toISOString(),
                        });
                        resetStreaming();
                    },
                    onError: (err) => {
                        if (!isActiveRequest()) return;
                        message.error(err.message || '回答生成失败');
                        resetStreaming();
                        loadMessages(sessionId);
                    },
                },
                controller.signal
            );
        } catch {
            if (isActiveRequest()) {
                resetStreaming();
            }
        } finally {
            const active = activeRequestRef.current;
            if (active?.sessionId === sessionId && active.generation === generation) {
                activeRequestRef.current = null;
            }
        }
    };

    const handleStop = () => {
        const active = activeRequestRef.current;
        if (!active) return;
        active.controller.abort();
        activeRequestRef.current = null;
        requestGenerationRef.current += 1;
        resetStreaming();
        message.info('已停止生成');
    };

    // 清空当前会话历史
    const handleClear = async () => {
        if (!currentSessionId) return;
        try {
            await clearQAMessages(currentSessionId);
            clearMessages();
            message.success('已清空');
        } catch {
            message.error('清空失败');
        }
    };

    const handleKeyDown = (e: React.KeyboardEvent) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            handleSend();
        }
    };

    return (
        <div className="flex h-full">
            {/* 左侧会话列表 */}
            <div className="w-64 flex flex-col border-r border-gray-200 bg-white">
                <div className="p-3 border-b border-gray-200">
                    <Button type="primary" icon={<PlusOutlined/>} block onClick={openPicker}>
                        新建问答
                    </Button>
                </div>
                <div className="flex-1 overflow-auto">
                    {loadingSessions ? (
                        <div className="flex justify-center p-4"><Spin/></div>
                    ) : sessions.length === 0 ? (
                        <div className="p-4 text-center text-gray-400 text-sm">暂无问答会话</div>
                    ) : (
                        sessions.map((s) => (
                            <div
                                key={s.id}
                                onClick={() => setCurrentSession(s.id)}
                                className={`group flex items-center justify-between px-3 py-2.5 cursor-pointer border-b border-gray-100 hover:bg-gray-50 ${
                                    currentSessionId === s.id ? 'bg-blue-50 border-l-2 border-l-[#2260f2]' : ''
                                }`}
                            >
                                <div className="flex-1 min-w-0">
                                    <div className="text-sm truncate">{s.title}</div>
                                    <div className="text-xs text-gray-400">{s.created_at}</div>
                                </div>
                                <Tooltip title="删除">
                                    <Button
                                        type="text"
                                        size="small"
                                        danger
                                        icon={<DeleteOutlined/>}
                                        onClick={(e) => {
                                            e.stopPropagation();
                                            handleDelete(s.id);
                                        }}
                                        className="opacity-0 group-hover:opacity-100"
                                    />
                                </Tooltip>
                            </div>
                        ))
                    )}
                </div>
            </div>

            {/* 右侧对话区 */}
            <div className="flex-1 flex flex-col bg-gray-50 min-w-0">
                {!currentSessionId ? (
                    <div className="flex-1 flex items-center justify-center">
                        <Empty description="请选择或新建问答会话"/>
                    </div>
                ) : (
                    <>
                        <div className="flex items-center justify-between px-4 py-2 bg-white border-b border-gray-200">
                            <span className="text-sm text-gray-500">基于已上传合同的智能问答（支持多轮）</span>
                            <Button
                                size="small"
                                icon={<ClearOutlined/>}
                                onClick={handleClear}
                                disabled={isStreaming || messages.length === 0}
                            >
                                清空
                            </Button>
                        </div>
                        <div className="flex-1 overflow-auto p-4 space-y-4">
                            {loadingMessages ? (
                                <div className="flex justify-center p-8"><Spin/></div>
                            ) : (
                                <>
                                    {messages.length === 0 && !isStreaming && (
                                        <div className="flex justify-center p-8">
                                            <Empty description="向合同提问，例如：这份合同有哪些违约风险？"/>
                                        </div>
                                    )}
                                    {messages.map((m) => (
                                        <MessageBubble key={m.id} role={m.role} content={m.content}/>
                                    ))}
                                    {isStreaming && (
                                        <MessageBubble role="assistant" content={streamingContent} streaming/>
                                    )}
                                    <div ref={messagesEndRef}/>
                                </>
                            )}
                        </div>
                        <div className="p-3 bg-white border-t border-gray-200">
                            <div className="flex gap-2 items-end">
                                <TextArea
                                    value={input}
                                    onChange={(e) => setInput(e.target.value)}
                                    onKeyDown={handleKeyDown}
                                    placeholder="针对合同内容提问，Enter 发送，Shift+Enter 换行"
                                    autoSize={{minRows: 1, maxRows: 4}}
                                    disabled={isStreaming}
                                    className="flex-1"
                                />
                                {isStreaming ? (
                                    <Button danger icon={<StopOutlined/>} onClick={handleStop}>
                                        停止
                                    </Button>
                                ) : (
                                    <Button
                                        type="primary"
                                        icon={<SendOutlined/>}
                                        onClick={handleSend}
                                        disabled={!input.trim()}
                                    >
                                        发送
                                    </Button>
                                )}
                            </div>
                        </div>
                    </>
                )}
            </div>

            {/* 合同选择 Modal */}
            <Modal
                title="选择合同开始问答"
                open={pickerOpen}
                onCancel={() => setPickerOpen(false)}
                onOk={handleCreate}
                okText="开始问答"
                cancelText="取消"
                confirmLoading={creating}
                okButtonProps={{disabled: !selectedFileId}}
            >
                {loadingContracts ? (
                    <div className="flex justify-center p-8"><Spin/></div>
                ) : contracts.length === 0 ? (
                    <Empty description="暂无可选合同，请先在「合同审阅」上传合同"/>
                ) : (
                    <Select
                        className="w-full"
                        placeholder="请选择一份合同"
                        value={selectedFileId}
                        onChange={(v) => setSelectedFileId(v)}
                        showSearch
                        optionFilterProp="label"
                        options={contracts.map((c) => ({
                            value: c.ID,
                            label: `${c.Title}${c.PartyA || c.PartyB ? `（${c.PartyA || '?'} - ${c.PartyB || '?'}）` : ''}`,
                        }))}
                    />
                )}
            </Modal>
        </div>
    );
}

function MessageBubble({role, content, streaming}: { role: string; content: string; streaming?: boolean }) {
    const isUser = role === 'user';
    return (
        <div className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}>
            <div
                className={`max-w-[75%] px-4 py-2.5 rounded-lg text-sm whitespace-pre-wrap break-words ${
                    isUser
                        ? 'bg-[#2260f2] text-white rounded-br-none'
                        : 'bg-white text-gray-800 border border-gray-200 rounded-bl-none'
                }`}
            >
                {content || (streaming ? '思考中…' : '')}
                {streaming && content && (
                    <span className="inline-block w-1.5 h-4 ml-0.5 bg-gray-400 animate-pulse align-middle"/>
                )}
            </div>
        </div>
    );
}
