import {create} from 'zustand/index';
import type {QAMessage, ListSessionResponse} from '@/lib/Interface';

interface QAState {
    sessions: ListSessionResponse[];
    currentSessionId: number | null;
    messages: QAMessage[];
    isStreaming: boolean;
    streamingContent: string;
    setSessions: (s: ListSessionResponse[]) => void;
    setCurrentSession: (id: number | null) => void;
    setMessages: (m: QAMessage[]) => void;
    appendMessage: (m: QAMessage) => void;
    startStreaming: () => void;
    setStreaming: (b: boolean) => void;
    appendDelta: (d: string) => void;
    resetStreaming: () => void;
    clearMessages: () => void;
}

export const useQAStore = create<QAState>((set) => ({
    sessions: [],
    currentSessionId: null,
    messages: [],
    isStreaming: false,
    streamingContent: '',
    setSessions: (sessions) => set({sessions}),
    setCurrentSession: (currentSessionId) => set({currentSessionId}),
    setMessages: (messages) => set({messages}),
    appendMessage: (m) => set((s) => ({messages: [...s.messages, m]})),
    startStreaming: () => set({isStreaming: true, streamingContent: ''}),
    setStreaming: (isStreaming) => set({isStreaming}),
    appendDelta: (d) => set((s) => ({streamingContent: s.streamingContent + d})),
    resetStreaming: () => set({streamingContent: '', isStreaming: false}),
    clearMessages: () => set({messages: []}),
}));
