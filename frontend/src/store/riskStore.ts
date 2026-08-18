"use client";
import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";
import { ReviewProgressEvent, RiskResponse } from "@/lib/Interface";

type RiskState = {
  riskDataList: RiskResponse[];
  replacedNum: number;
  isStreaming: boolean;
  isCompleted: boolean;
  currentProgress: ReviewProgressEvent | null;
  progressEvents: ReviewProgressEvent[];
  sourceFileUrl: string | null; // 记录当前风险点对应的文档 URL
  addRiskData: (risk: RiskResponse) => void;
  setRiskDataList: (riskList: RiskResponse[], sourceFileUrl?: string) => void;
  updateRiskAccepted: (id: number, isAccepted: boolean) => void;
  updateRiskContent: (id: number, updates: Partial<RiskResponse>) => void;
  removeRiskData: (id: number) => void;
  addReplacedNum: () => void;
  resetRiskData: () => void;
  setStreaming: (isStreaming: boolean) => void;
  setCompleted: (isCompleted: boolean) => void;
  addProgressEvent: (event: ReviewProgressEvent) => void;
  setSourceFileUrl: (url: string | null) => void;
};

const STORAGE_KEY = "risk-store";

// overwriteRisk 后端按 stableKey 去重后发送完整记录，同 id 视为更新而非增量；
// 直接覆盖旧值，仅对空字段保留旧值，避免空串覆盖已有内容。
const overwriteRisk = (old: RiskResponse, next: RiskResponse): RiskResponse => ({
  ...old,
  ...next,
  original_content: next.original_content || old.original_content,
  risk_analysis: next.risk_analysis || old.risk_analysis,
  risk_level: next.risk_level || old.risk_level,
  risk_type: next.risk_type || old.risk_type,
  suggested_content: next.suggested_content || old.suggested_content,
  reason: next.reason || old.reason,
  is_accepted: next.is_accepted ?? old.is_accepted,
  created_at: next.created_at || old.created_at,
});

// debouncedStorage 包装 localStorage：每条风险点更新都同步 JSON.stringify+setItem
// 会阻塞主线程；改为 1s 防抖合并写入，页面卸载前立即冲刷待写，保证最终一致性。
const pendingWrites: Record<string, { value: string; timer: ReturnType<typeof setTimeout> | null }> = {};

const flushWrite = (name: string) => {
  const entry = pendingWrites[name];
  if (!entry) return;
  if (entry.timer) {
    clearTimeout(entry.timer);
    entry.timer = null;
  }
  try {
    localStorage.setItem(name, entry.value);
  } catch {
    // 忽略配额超限等写入失败
  }
  delete pendingWrites[name];
};

if (typeof window !== "undefined") {
  window.addEventListener("beforeunload", () => {
    Object.keys(pendingWrites).forEach(flushWrite);
  });
}

const debouncedStorage = {
  getItem: (name: string) => localStorage.getItem(name),
  setItem: (name: string, value: string) => {
    const entry = pendingWrites[name] || { value, timer: null };
    entry.value = value;
    if (entry.timer) clearTimeout(entry.timer);
    entry.timer = setTimeout(() => flushWrite(name), 1000);
    pendingWrites[name] = entry;
  },
  removeItem: (name: string) => {
    if (pendingWrites[name]) {
      clearTimeout(pendingWrites[name].timer as unknown as number);
      delete pendingWrites[name];
    }
    localStorage.removeItem(name);
  },
};

export const RiskStore = create<RiskState>()(
  persist(
    (set) => ({
      riskDataList: [],
      replacedNum: 0,
      isStreaming: false,
      isCompleted: false,
      currentProgress: null,
      progressEvents: [],
      sourceFileUrl: null,
      addRiskData: (risk) =>
        set((state) => {
          const exists = state.riskDataList.some((item) => item.id === risk.id);
          if (!exists) {
            return { riskDataList: [...state.riskDataList, risk] };
          }
          return {
            riskDataList: state.riskDataList.map((item) =>
              item.id === risk.id ? overwriteRisk(item, risk) : item
            ),
          };
        }),
      setRiskDataList: (riskList, sourceFileUrl) => set({
        riskDataList: riskList,
        sourceFileUrl: sourceFileUrl ?? null,
      }),
      updateRiskAccepted: (id, isAccepted) =>
        set((state) => ({
          riskDataList: state.riskDataList.map((item) =>
            item.id === id ? { ...item, is_accepted: isAccepted } : item
          ),
        })),
      updateRiskContent: (id, updates) =>
        set((state) => ({
          riskDataList: state.riskDataList.map((item) =>
            item.id === id ? { ...item, ...updates } : item
          ),
        })),
      removeRiskData: (id) =>
        set((state) => ({
          riskDataList: state.riskDataList.filter((item) => item.id !== id),
        })),
      addReplacedNum: () =>
        set((state) => ({ replacedNum: state.replacedNum + 1 }))
      ,
      resetRiskData: () =>
        set({ riskDataList: [],replacedNum: 0, isStreaming: false, isCompleted: false, currentProgress: null, progressEvents: [], sourceFileUrl: null }),
      setStreaming: (isStreaming) => set({ isStreaming }),
      setCompleted: (isCompleted) => set({ isCompleted }),
      addProgressEvent: (event) =>
        set((state) => {
          const eventProgress = Number(event.progress || 0);
          const isTerminalEvent =
            event.status === "failed" ||
            eventProgress >= 1 ||
            (event.phase === "report" && event.status === "completed");

          return {
            currentProgress: event,
            progressEvents: [...state.progressEvents, event].slice(-30),
            isStreaming: isTerminalEvent ? false : true,
            isCompleted: isTerminalEvent && event.status !== "failed" ? true : state.isCompleted,
          };
        }),
      setSourceFileUrl: (url) => set({ sourceFileUrl: url }),
    }),
    {
      name: STORAGE_KEY,
      storage: createJSONStorage(() => debouncedStorage),
      partialize: (state) => ({
        riskDataList: state.riskDataList,
        isCompleted: state.isCompleted,
        isStreaming: state.isStreaming,
        replacedNum: state.replacedNum,
        sourceFileUrl: state.sourceFileUrl,
      }),
    }
  )
);
