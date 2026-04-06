import {getAuthToken, clearTokenInfo} from "@/utils/client";
import {RiskResponse} from "@/lib/Interface";
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
    onCompleted?: () => void
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
        let processPromise = Promise.resolve();

        try {
            while (true) {
                const {done, value} = await reader.read();

                if (done) {
                    if (buffer.trim()) {
                        const lines = buffer.split("\n");
                        for (const line of lines) {
                            const trimmedLine = line.trim();
                            if (trimmedLine && trimmedLine.startsWith("data:")) {
                                const dataContent = trimmedLine.slice(5).trim();
                                if (dataContent) {
                                    processPromise = processPromise.then(() => {
                                        return new Promise<void>((resolve) => {
                                            requestAnimationFrame(() => {
                                                processDataLine(dataContent, onRiskData);
                                                setTimeout(() => {
                                                    resolve();
                                                }, 200);
                                            });
                                        });
                                    });
                                }
                            }
                        }
                    }
                    await processPromise;
                    onStreamingEnd?.();
                    onCompleted?.();
                    onComplete?.();
                    break;
                }

                buffer += decoder.decode(value, {stream: true});

                const lines = buffer.split("\n");
                buffer = lines.pop() || "";

                for (const line of lines) {
                    const trimmedLine = line.trim();

                    if (!trimmedLine || trimmedLine.startsWith(": ping")) {
                        continue;
                    }

                    if (trimmedLine.startsWith("data:")) {
                        const dataContent = trimmedLine.slice(5).trim();
                        if (dataContent) {
                            processPromise = processPromise.then(() => {
                                return new Promise<void>((resolve) => {
                                    requestAnimationFrame(() => {
                                        processDataLine(dataContent, onRiskData);
                                        setTimeout(() => {
                                            resolve();
                                        }, 200);
                                    });
                                });
                            });
                        }
                    }
                }
            }
        } catch (error) {
            onStreamingEnd?.();
            onError?.(error as Error);
        }
    }

    return {connected: true};
};

function processDataLine(
    dataContent: string,
    onRiskData?: (risk: RiskResponse) => void
) {
    if (!dataContent) {
        return;
    }

    try {
        const parsed = JSON.parse(dataContent);

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
                    suggested_content: riskDataObj.suggested_content || "",
                    is_accepted: isAccepted,
                    created_at: riskDataObj.created_at || "",
                };

                onRiskData?.(riskData);
            }
        }
    } catch (error) {
    }
}

function processSSEData(
    eventText: string,
    onRiskData?: (risk: RiskResponse) => void
) {
    const lines = eventText.split("\n");
    let eventType = "";
    let dataText = "";

    for (const line of lines) {
        if (line.startsWith("event:")) {
            eventType = line.slice(6).trim();
        } else if (line.startsWith("data:")) {
            const dataLine = line.slice(5).trim();
            if (dataText) {
                dataText += dataLine;
            } else {
                dataText = dataLine;
            }
        }
    }

    if (!dataText) return;

    const parseJsonData = (jsonStr: string) => {
        let startIndex = 0;
        const results: Array<{
            event?: string;
            data?: unknown;
            id?: unknown;
            session_id?: unknown;
            index?: unknown;
            [key: string]: unknown;
        }> = [];

        while (startIndex < jsonStr.length) {
            let braceCount = 0;
            let inString = false;
            let escapeNext = false;
            let jsonStart = -1;

            for (let i = startIndex; i < jsonStr.length; i++) {
                const char = jsonStr[i];

                if (escapeNext) {
                    escapeNext = false;
                    continue;
                }

                if (char === "\\") {
                    escapeNext = true;
                    continue;
                }

                if (char === '"') {
                    inString = !inString;
                    continue;
                }

                if (!inString) {
                    if (char === "{") {
                        if (jsonStart === -1) {
                            jsonStart = i;
                        }
                        braceCount++;
                    } else if (char === "}") {
                        braceCount--;
                        if (braceCount === 0 && jsonStart !== -1) {
                            try {
                                const jsonSubStr = jsonStr.substring(jsonStart, i + 1);
                                const parsed = JSON.parse(jsonSubStr);
                                results.push(parsed);
                                startIndex = i + 1;
                                break;
                            } catch (e) {
                                startIndex = i + 1;
                                break;
                            }
                        }
                    }
                }
            }

            if (braceCount !== 0) {
                break;
            }
        }

        return results;
    };

    try {
        const parsedObjects = parseJsonData(dataText);

        parsedObjects.forEach((parsed, index) => {
            setTimeout(() => {
                let riskDataObj: RiskResponse | null = null;

                if (parsed.event === "message" && parsed.data) {
                    riskDataObj = parsed.data as RiskResponse;
                } else if (parsed.event === "end") {
                    return;
                } else if (eventType === "message" && parsed.id && parsed.session_id) {
                    riskDataObj = parsed as unknown as RiskResponse;
                } else if (eventType === "message" && parsed.data) {
                    riskDataObj = parsed.data as RiskResponse;
                } else if (
                    parsed.id &&
                    parsed.session_id &&
                    parsed.index !== undefined
                ) {
                    riskDataObj = parsed as unknown as RiskResponse;
                }

                if (
                    riskDataObj &&
                    riskDataObj.id &&
                    riskDataObj.session_id &&
                    riskDataObj.index !== undefined
                ) {
                    const isAcceptedValue = (
                        riskDataObj as unknown as { is_accepted?: boolean | number }
                    ).is_accepted;
                    const isAccepted =
                        typeof isAcceptedValue === "number"
                            ? isAcceptedValue === 1
                            : Boolean(isAcceptedValue);

                    const riskData: RiskResponse = {
                        id: riskDataObj.id,
                        session_id: riskDataObj.session_id,
                        task_id: riskDataObj.task_id,
                        index: riskDataObj.index,
                        original_content: riskDataObj.original_content,
                        risk_analysis: riskDataObj.risk_analysis,
                        risk_level: riskDataObj.risk_level,
                        suggested_content: riskDataObj.suggested_content,
                        is_accepted: isAccepted,
                        created_at: riskDataObj.created_at,
                    };
                    onRiskData?.(riskData);
                }
            }, index * 50);
        });
    } catch (error) {
    }
}
