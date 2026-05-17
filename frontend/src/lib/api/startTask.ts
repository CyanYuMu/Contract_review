import {getAuthToken, clearTokenInfo} from "@/utils/client";
import {ReviewProgressEvent, RiskResponse} from "@/lib/Interface";
import {authDatedHandler} from "@/utils/authDatedHandler";

export type ReviewTaskCreateRequest = {
    session_id: number;
    stance: string;
    contract_type: string;
    description?: string | null;
    intensity: string;
};

export const startTask = async (
    data: ReviewTaskCreateRequest,
    onConnected?: () => void,
    onRiskData?: (risk: RiskResponse) => void,
    onComplete?: () => void,
    onError?: (error: Error) => void,
    onStreamingStart?: () => void,
    onStreamingEnd?: () => void,
    onCompleted?: () => void,
    onProgress?: (event: ReviewProgressEvent) => void
) => {
    const token = getAuthToken();

    const response = await fetch(`/api/proxy/review_task/start_task`, {
        method: "POST",
        body: JSON.stringify(data),
        headers: {
            "Content-Type": "application/json",
            ...(token && {Authorization: `Bearer ${token}`}),
            Accept: "text/event-stream",
        },
    });

    if (!response.ok) {
        // 处理401/403错误：清空用户信息并弹出登录过期提示
        if (response.status === 401 || response.status === 403) {
            clearTokenInfo();
            authDatedHandler.trigger403Error();
        }
        const text = await response.text().catch(() => "");
        const error = new Error(
            text || `Request failed: ${response.status} ${response.statusText}`
        );
        onError?.(error);
        onStreamingEnd?.();
        throw error;
    }

    if (onConnected) {
        onConnected();
    }

    onStreamingStart?.();

    if (response.body) {
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        let finalized = false;

        const finalizeStream = () => {
            if (finalized) return;
            finalized = true;
            onStreamingEnd?.();
            onCompleted?.();
            onComplete?.();
        };

        const handleEventText = (eventText: string): boolean => {
            const result = processSSEEvent(eventText, onRiskData, onProgress);
            if (result.error) {
                throw result.error;
            }
            if (result.complete) {
                finalizeStream();
                return true;
            }
            return false;
        };

        try {
            while (true) {
                const {done, value} = await reader.read();

                if (done) {
                    const remaining = buffer + decoder.decode();
                    if (remaining.trim()) {
                        handleEventText(remaining);
                    }
                    finalizeStream();
                    break;
                }

                buffer += decoder.decode(value, {stream: true});

                const events = buffer.split(/\r?\n\r?\n/);
                buffer = events.pop() || "";

                for (const eventText of events) {
                    if (handleEventText(eventText)) {
                        await reader.cancel().catch(() => undefined);
                        return {connected: true};
                    }
                }
            }
        } catch (error) {
            if (!finalized) {
                onStreamingEnd?.();
            }
            onError?.(error as Error);
            throw error;
        }
    }

    return {connected: true};
};

function processSSEEvent(
    eventText: string,
    onRiskData?: (risk: RiskResponse) => void,
    onProgress?: (event: ReviewProgressEvent) => void
): { error?: Error; complete?: boolean } {
    const dataLines = eventText
        .split(/\r?\n/)
        .map((line) => line.trim())
        .filter((line) => line && !line.startsWith(":"))
        .filter((line) => line.startsWith("data:"))
        .map((line) => line.slice(5).trim());

    if (dataLines.length === 0) {
        return {};
    }

    for (const dataContent of dataLines) {
        const result = processDataLine(dataContent, onRiskData, onProgress);
        if (result.error || result.complete) {
            return result;
        }
    }

    return {};
}

function processDataLine(
    dataContent: string,
    onRiskData?: (risk: RiskResponse) => void,
    onProgress?: (event: ReviewProgressEvent) => void
): { error?: Error; complete?: boolean } {
    if (!dataContent) {
        return {};
    }

    try {
        const parsed = JSON.parse(dataContent);

        if (parsed.event === "error") {
            const message = parsed.data?.message || "审阅任务启动失败";
            return {error: new Error(message)};
        }

        if (parsed.event === "end") {
            return {complete: true};
        }

        if (parsed.event === "progress" && parsed.data) {
            const progressData = parsed.data;
            onProgress?.({
                phase: progressData.phase || "",
                agent: progressData.agent || "",
                status: progressData.status || "",
                message: progressData.message || "",
                progress: Number(progressData.progress || 0),
                timestamp: progressData.timestamp || "",
                data: progressData.data,
            });
            return {};
        }

        if (parsed.event === "message" && parsed.data) {
            const riskDataObj = parsed.data;

            if (
                riskDataObj.id &&
                riskDataObj.session_id &&
                riskDataObj.index !== undefined
            ) {
                const isAcceptedValue = riskDataObj.is_accepted;
                const isAccepted =
                    typeof isAcceptedValue === "number"
                        ? isAcceptedValue === 1
                        : Boolean(isAcceptedValue);

                const riskData: RiskResponse = {
                    id: riskDataObj.id,
                    session_id: riskDataObj.session_id,
                    task_id: riskDataObj.task_id,
                    index: riskDataObj.index,
                    original_content: riskDataObj.original_content || "",
                    risk_analysis: riskDataObj.risk_analysis || "",
                    risk_level: riskDataObj.risk_level || "",
                    risk_type: riskDataObj.risk_type || "",
                    suggested_content: riskDataObj.suggested_content || "",
                    reason: riskDataObj.reason || "",
                    is_accepted: isAccepted,
                    created_at: riskDataObj.created_at || "",
                };

                onRiskData?.(riskData);
            }
        }
    } catch {
    }

    return {};
}
