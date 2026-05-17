"use client";
import { create } from "zustand";
import { persist } from "zustand/middleware";
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
          if (exists) {
            const newList = state.riskDataList.map((item) => {
              if (item.id === risk.id) {
                const mergeText = (oldText: string, newText: string) => {
                  if (!newText) return oldText;
                  if (!oldText) return newText;

                  if (
                    newText.includes(oldText) &&
                    newText.length > oldText.length
                  ) {
                    return newText;
                  }

                  if (
                    oldText.includes(newText) &&
                    oldText.length > newText.length
                  ) {
                    return oldText;
                  }

                  const overlapLength = Math.min(
                    20,
                    Math.min(oldText.length, newText.length)
                  );
                  const oldEnd = oldText.slice(-overlapLength);
                  const newStart = newText.slice(0, overlapLength);

                  if (oldEnd && newStart && oldEnd === newStart) {
                    return oldText + newText.slice(overlapLength);
                  }

                  if (
                    newText.length > oldText.length &&
                    newText.startsWith(
                      oldText.slice(0, Math.min(10, oldText.length))
                    )
                  ) {
                    return newText;
                  }

                  return newText;
                };

                return {
                  ...item,
                  original_content: mergeText(
                    item.original_content || "",
                    risk.original_content || ""
                  ),
                  risk_analysis: mergeText(
                    item.risk_analysis || "",
                    risk.risk_analysis || ""
                  ),
                  suggested_content: mergeText(
                    item.suggested_content || "",
                    risk.suggested_content || ""
                  ),
                  reason: mergeText(item.reason || "", risk.reason || ""),
                  risk_level: risk.risk_level || item.risk_level,
                  risk_type: risk.risk_type || item.risk_type,
                  is_accepted: risk.is_accepted ?? item.is_accepted,
                  created_at: risk.created_at || item.created_at,
                };
              }
              return item;
            });
            return { riskDataList: newList };
          }
          return {
            riskDataList: [...state.riskDataList, risk],
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
      partialize: (state) => ({
        riskDataList: state.riskDataList,
        isCompleted: state.isCompleted,
        sourceFileUrl: state.sourceFileUrl,
      }),
    }
  )
);
