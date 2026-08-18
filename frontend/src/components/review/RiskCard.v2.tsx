import React, {useEffect, useMemo, useRef, useState} from "react";
import {Button, Modal, Switch} from "antd";
import toast from "react-hot-toast";
import {ReviewProgressEvent, RiskResponse} from "@/lib/Interface";
import Editor, {type IElement} from "@/lib/canvas-editor/editor";
import {removeQuotes} from "@/utils/textUtils";
import {CharacterMapper} from "@/utils/characterMapping";
import {RiskStore} from "@/store/riskStore";
import "./reviewing-animation.css";
import {assets} from "@/assets/assets";
import Image from "next/image";
import {contractAccept} from "@/lib/api/contractAccept";
import {getContrastStatus} from "@/lib/api/getContrastStatus";
type RiskCardProps = {
    riskDataList?: RiskResponse[];
    editor?: InstanceType<typeof Editor> | null;
};

const phaseLabelMap: Record<string, string> = {
    prepare: "准备审阅",
    clause_split: "条款拆分",
    candidate_retrieve: "依据命中",
    risk_identify: "风险识别",
    suggestion: "修改建议",
    quality: "质量评估",
    report: "报告生成",
};

function getPhaseLabel(phase: string) {
    return phaseLabelMap[phase] || phase || "审阅进度";
}

function extractCriticalGaps(event: ReviewProgressEvent | null): string[] {
    if (!event?.data || typeof event.data !== "object") return [];

    const data = event.data as {
        critical_gaps?: unknown;
        criticalGaps?: unknown;
        CriticalGaps?: unknown;
    };
    const gaps = data.critical_gaps ?? data.criticalGaps ?? data.CriticalGaps;
    if (!Array.isArray(gaps)) return [];

    return gaps
        .filter((item): item is string => typeof item === "string")
        .slice(0, 3);
}

function extractQualityScore(event: ReviewProgressEvent | null): number | null {
    if (!event?.data || typeof event.data !== "object") return null;

    const data = event.data as { overall_score?: unknown; overallScore?: unknown };
    const score = data.overall_score ?? data.overallScore;
    return typeof score === "number" ? score : null;
}

function formatEventMeta(event: ReviewProgressEvent | null): string {
    if (!event?.data || typeof event.data !== "object") return "";

    const data = event.data as {
        event_type?: unknown;
        legal_basis_count?: unknown;
        legalBasisCount?: unknown;
        legal_basis_sources?: unknown;
        candidate_count?: unknown;
        candidate_ids?: unknown;
        candidate_sources?: unknown;
        risk_type?: unknown;
        risk_level?: unknown;
        finding_count?: unknown;
        verified_count?: unknown;
        total?: unknown;
        verified?: unknown;
        clause_count?: unknown;
        suggestion_count?: unknown;
    };

    if (data.event_type === "risk_found") {
        const count = Number(data.legal_basis_count ?? data.legalBasisCount ?? 0);
        const sources = Array.isArray(data.legal_basis_sources)
            ? data.legal_basis_sources.filter((item): item is string => typeof item === "string").slice(0, 2)
            : [];
        const status = data.verified === false ? "待人工确认" : "已验证";
        return [
            status,
            data.risk_type ? String(data.risk_type) : "",
            data.risk_level ? String(data.risk_level) : "",
            `依据命中 ${count} 条${sources.length ? `：${sources.join("、")}` : ""}`,
        ].filter(Boolean).join(" · ");
    }

    if (data.event_type === "candidate_retrieved") {
        const count = Number(data.candidate_count ?? 0);
        const sources = Array.isArray(data.candidate_sources)
            ? data.candidate_sources.filter((item): item is string => typeof item === "string").slice(0, 3)
            : [];
        const ids = Array.isArray(data.candidate_ids)
            ? data.candidate_ids.filter((item): item is string => typeof item === "string").slice(0, 3)
            : [];
        const sourceText = sources.length ? `：${sources.join("、")}` : "";
        const idText = ids.length ? ` (${ids.join("、")})` : "";
        return `候选风险点 ${count} 条${sourceText}${idText}`;
    }

    if (data.event_type === "clause_reviewed") {
        const candidateCount = Number(data.candidate_count ?? 0);
        const sources = Array.isArray(data.candidate_sources)
            ? data.candidate_sources.filter((item): item is string => typeof item === "string").slice(0, 2)
            : [];
        const evidenceText = candidateCount > 0
            ? `，候选 ${candidateCount} 条${sources.length ? `：${sources.join("、")}` : ""}`
            : "";
        return `本条发现 ${Number(data.finding_count ?? 0)} 个风险点，已验证 ${Number(data.verified_count ?? 0)} 个${evidenceText}`;
    }

    const total = data.total;
    const verified = data.verified_count ?? data.verified;
    if (typeof total === "number" && typeof verified === "number") {
        return `已验证 ${verified}/${total}`;
    }

    if (typeof data.clause_count === "number") {
        return `条款 ${data.clause_count} 个${typeof data.suggestion_count === "number" ? `，建议 ${data.suggestion_count} 条` : ""}`;
    }

    return "";
}

function ReviewProgressPanel({
                                 currentProgress,
                                 progressEvents,
                             }: {
    currentProgress: ReviewProgressEvent | null;
    progressEvents: ReviewProgressEvent[];
}) {
    const percent = Math.round((currentProgress?.progress ?? 0) * 100);
    const gaps = extractCriticalGaps(currentProgress);
    const qualityScore = extractQualityScore(currentProgress);
    const currentMeta = formatEventMeta(currentProgress);
    const events = [...progressEvents].reverse();

    return (
        <div className="review-progress-panel">
            <div className="review-progress-header">
                <div>
                    <div className="review-progress-title">
                        {getPhaseLabel(currentProgress?.phase || "")}
                    </div>
                    <div className="review-progress-message">
                        {currentProgress?.message || "正在启动合同审阅..."}
                    </div>
                </div>
                <div className="review-progress-metrics">
                    <div className="review-progress-percent">{percent}%</div>
                    {qualityScore !== null && (
                        <div className="review-progress-score">
                            质量 {qualityScore.toFixed(2)}
                        </div>
                    )}
                </div>
            </div>

            <div className="review-progress-track">
                <div
                    className="review-progress-fill"
                    style={{width: `${Math.min(100, Math.max(0, percent))}%`}}
                />
            </div>

            {gaps.length > 0 && (
                <div className="review-progress-gaps">
                    {gaps.map((gap) => (
                        <div key={gap} className="review-progress-gap">
                            {gap}
                        </div>
                    ))}
                </div>
            )}

            {currentMeta && (
                <div className="review-progress-meta">
                    {currentMeta}
                </div>
            )}

            {events.length > 0 && (
                <div className="review-progress-timeline">
                    {events.map((event, index) => (
                        <div
                            key={`${event.phase}-${event.status}-${event.timestamp}-${index}`}
                            className={`review-progress-step ${event.status === "completed" ? "is-completed" : ""}`}
                        >
                            <span className="review-progress-dot"/>
                            <div className="review-progress-step-content">
                                <div className="review-progress-step-title">
                                    {getPhaseLabel(event.phase)}
                                    {event.agent ? ` · ${event.agent}` : ""}
                                </div>
                                <div className="review-progress-step-message">{event.message}</div>
                                {formatEventMeta(event) && (
                                    <div className="review-progress-step-meta">{formatEventMeta(event)}</div>
                                )}
                            </div>
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}

/**
 * 从 HTML 字符串中提取纯文本
 */
function extractTextFromHTML(html: string): string {
    if (!html) return "";

    // 如果不包含 HTML 标签，直接返回
    if (!/<[^>]+>/.test(html)) {
        return html;
    }

    // 创建临时 DOM 元素来解析 HTML
    const tempDiv = document.createElement("div");
    tempDiv.innerHTML = html;

    // 获取纯文本内容
    const text = tempDiv.textContent || tempDiv.innerText || "";

    // 清理临时元素
    tempDiv.remove();

    return text;
}

function normalizeRiskText(content: string): string {
    return removeQuotes(extractTextFromHTML(content))
        .replace(/\u00a0/g, " ")
        .replace(/[ \t]+/g, " ")
        .replace(/\s*\n\s*/g, "\n")
        .trim();
}

function buildEditorSearchTexts(searchText: string, allowSnippets = false): string[] {
    const normalized = normalizeRiskText(searchText);
    const candidates = [
        normalized,
        normalized.replace(/\s+/g, " ").trim(),
        normalized.replace(/[\r\n]+/g, "").trim(),
        normalized.replace(/\s*([，。！？；：、,.!?;:])\s*/g, "$1").trim(),
    ];

    if (allowSnippets && normalized.length > 80) {
        candidates.push(
            normalized.slice(0, 120).trim(),
            normalized.slice(-120).trim(),
            normalized.slice(0, 80).trim(),
            normalized.slice(-80).trim()
        );
    }

    const seen = new Set<string>();
    return candidates.filter((item) => {
        if (!item || item.length < 4 || seen.has(item)) return false;
        seen.add(item);
        return true;
    });
}

function pickBaseFormat(selectionElementList: Array<Partial<IElement>> = []) {
    const firstElement =
        selectionElementList.find((item) => item?.value && item.value !== "\n") ||
        selectionElementList[0] ||
        {};

    return {
        font: firstElement.font,
        size: firstElement.size,
        bold: firstElement.bold,
        italic: firstElement.italic,
        underline: firstElement.underline,
        strikeout: firstElement.strikeout,
        rowFlex: firstElement.rowFlex,
        rowMargin: firstElement.rowMargin,
        letterSpacing: firstElement.letterSpacing,
        highlight: firstElement.highlight,
        textDecoration: firstElement.textDecoration,
    };
}

export default function RiskCard({riskDataList: propRiskDataList = [], editor}: RiskCardProps) {
    const [selectedId, setSelectedId] = useState<number | null>(null);
    const [modalContent, setModalContent] = useState<string>("");
    const [isModalVisible, setIsModalVisible] = useState(false);
    const mapperRef = useRef<CharacterMapper | null>(null);
    const containerRef = useRef<HTMLElement | null>(null);
    const [revise,setRevise] = useState(false);
    const [isReviseModalOpen, setIsReviseModalOpen] = useState(false);
    const [isCompletedModalOpen, setIsCompletedModalOpen] = useState(false);
    const prevIsStreamingRef = useRef<boolean | null>(null);
    // 获取删除函数和状态
    const removeRiskData = RiskStore((state) => state.removeRiskData);
    const addReplacedNum = RiskStore((state) => state.addReplacedNum);
    const replacedNum = RiskStore((state) => state.replacedNum);
    const isCompleted = RiskStore((state) => state.isCompleted);
    const isStreaming = RiskStore((state) => state.isStreaming);
    const currentProgress = RiskStore((state) => state.currentProgress);
    const progressEvents = RiskStore((state) => state.progressEvents);
    const storeRiskDataList = RiskStore((state) => state.riskDataList);
    const riskDataList =
        storeRiskDataList.length > 0 ? storeRiskDataList : propRiskDataList;

    // 显示 Modal
    const showModal = (content: string) => {
        setModalContent(content);
        setIsModalVisible(true);
    };

    // 复制原文到剪贴板
    const handleCopyText = () => {
        navigator.clipboard
            .writeText(modalContent)
            .then(() => {
                toast.success("已复制到剪贴板");
                setIsModalVisible(false);
            })
            .catch(() => {
                toast.error("复制失败");
            });
    };

    // 关闭 Modal
    const handleCloseModal = () => {
        setIsModalVisible(false);
    };

    const handleReviseOk = async () => {
        const fileId = Number(localStorage.getItem("uploaded_file_id") || 0);
        if (!fileId) {
            toast.error("未找到合同ID，请先上传合同");
            return;
        }
        try {
            await contractAccept({
                file_id: fileId,
                is_accepted: true,
            });
            setRevise(true);
            setIsReviseModalOpen(false);
            toast.success("已标记为已修订");
        } catch (error) {
            const message =
                error instanceof Error ? error.message : "状态修改失败";
            toast.error(message);
        }
    };

    const handleReviseCancel = () => {
        setIsReviseModalOpen(false);
        setRevise(false);
    };
    // 监听审查完成状态，弹出完成提示 Modal
    useEffect(() => {
        // 当 isStreaming 从 true 变为 false，且 isCompleted 为 true 时，表示审查刚完成
        if (prevIsStreamingRef.current === true && !isStreaming && isCompleted) {
            setIsCompletedModalOpen(true);
        }
        prevIsStreamingRef.current = isStreaming;
    }, [isStreaming, isCompleted]);

    // 初始化字符映射器
    useEffect(() => {
        if (!editor) return;

        let mounted = true;
        let retryCount = 0;
        const maxRetries = 20;
        const retryInterval = 100;
        const file_id=Number(localStorage.getItem("uploaded_file_id"));
        getContrastStatus(file_id).then((a)=>setRevise(a));
        const initMapper = () => {
            if (!mounted) return;

            const canvasContainer = document.querySelector(
                '[id^="canvasEditor-"]'
            ) as HTMLElement;

            // 检查容器是否存在且有足够内容
            const hasContent =
                canvasContainer &&
                canvasContainer.textContent &&
                canvasContainer.textContent.trim().length > 10;

            if (!hasContent) {
                retryCount++;
                if (retryCount < maxRetries) {
                    setTimeout(initMapper, retryInterval);
                    return;
                } else {
                    return;
                }
            }

            // 容器已准备好，初始化映射器
            containerRef.current = canvasContainer;
            mapperRef.current = new CharacterMapper(canvasContainer);
        };

        // 使用 requestAnimationFrame 确保 DOM 已渲染
        requestAnimationFrame(() => {
            requestAnimationFrame(() => {
                initMapper();
            });
        });

        return () => {
            mounted = false;
        };
    }, [editor]);

    const handleLocate = async (originalContent: string) => {
        if (!editor) {
            toast.error("编辑器尚未加载完成");
            return;
        }

        try {
            const cleanedOriginal = normalizeRiskText(originalContent);
            if (!cleanedOriginal) {
                toast.error("风险原文为空，无法定位");
                return;
            }

            // 优先级1: 优先使用编辑器 API
            const editorResult = await tryEditorAPI(editor, cleanedOriginal);
            if (editorResult) {
                return;
            }

            // 优先级2: 使用字符映射器
            if (mapperRef.current) {
                const mapperResult = tryCharacterMapper(
                    mapperRef.current,
                    cleanedOriginal
                );
                if (mapperResult) {
                    return;
                }
            }

            // 优先级3: 降级到传统 DOM 搜索
            const domResult = await tryDOMSearch(cleanedOriginal);
            if (domResult) {
                return;
            }

            // 所有方法都失败
            showModal(cleanedOriginal);
        } catch {
            toast.error("定位过程中发生错误，请稍后重试");
        }
    };

    const handleIgnore = (riskId: number) => {
        removeRiskData(riskId);
        toast.success("已忽略该风险点");
    };

    const handleReplace = async (
        riskId: number,
        originalContent: string,
        suggestedContent: string
    ) => {
        if (!editor) {
            toast.error("编辑器尚未加载完成");
            return;
        }

        try {
            const cleanedOriginal = normalizeRiskText(originalContent);
            const cleanedSuggestion = normalizeRiskText(suggestedContent);

            if (!cleanedOriginal || !cleanedSuggestion) {
                toast.error("缺少可应用的原文或修订建议");
                return;
            }

            // 优先级1: 优先使用编辑器 API
            const editorResult = await tryEditorAPIReplace(
                editor,
                cleanedOriginal,
                cleanedSuggestion
            );
            if (editorResult) {
                // 修订完成后，删除这条风险数据
                removeRiskData(riskId);
                // 已修订数量加1
                addReplacedNum()
                toast.success("修订成功");

                // 替换后重建映射
                if (mapperRef.current) {
                    setTimeout(() => {
                        mapperRef.current?.rebuild();
                    }, 300);
                }
                return;
            }

            // 优先级2: docx 导入渲染为 contenteditable HTML，编辑器元素列表为空，
            // 改用字符映射器在 DOM 中定位并替换。
            if (mapperRef.current) {
                const mapperReplaced = tryCharacterMapperReplace(
                    mapperRef.current,
                    cleanedOriginal,
                    cleanedSuggestion
                );
                if (mapperReplaced) {
                    removeRiskData(riskId);
                    addReplacedNum();
                    toast.success("修订成功");
                    return;
                }
            }

            // 定位失败：弹出原文，便于人工查找
            showModal(cleanedOriginal);
            toast.error("未能安全定位原文，已停止自动修订");
        } catch {
            toast.error("替换操作失败，请稍后重试");
        }
    };

    const sortedRiskData = useMemo(
        () => [...riskDataList].sort((a, b) => a.index - b.index),
        [riskDataList]
    );
    const riskOverview = useMemo(() => {
        const levelCount = {high: 0, medium: 0, low: 0};
        const typeCount = new Map<string, number>();
        sortedRiskData.forEach((item) => {
            if (item.risk_level?.includes("高")) levelCount.high += 1;
            else if (item.risk_level?.includes("低")) levelCount.low += 1;
            else levelCount.medium += 1;

            const type = item.risk_type || "未分类";
            typeCount.set(type, (typeCount.get(type) || 0) + 1);
        });
        return {
            levelCount,
            topTypes: Array.from(typeCount.entries())
                .sort((a, b) => b[1] - a[1])
                .slice(0, 3),
        };
    }, [sortedRiskData]);
    const totalRiskCount = sortedRiskData.length || 1;
    const hasReviewActivity = isStreaming || progressEvents.length > 0;

    // 空列表处理：区分"审查中"和"已全部修订"
    if (riskDataList.length === 0) {
        // 正在审查中（还没有数据）
        if (isStreaming || (hasReviewActivity && !isCompleted)) {
            return (
                <div className="reviewing-wrapper">
                    <div className="reviewing-container">
                        <div className="reviewing-spinner"></div>
                        <ReviewProgressPanel
                            currentProgress={currentProgress}
                            progressEvents={progressEvents}
                        />
                    </div>
                </div>
            );
        }

        if (!hasReviewActivity) {
            return (
                <div className="flex flex-col items-center justify-center p-12 text-center">
                    <div className="text-xl font-medium text-gray-800 mb-2">
                        暂无审阅结果
                    </div>
                    <div className="text-sm text-gray-500">请先上传合同并启动审查</div>
                </div>
            );
        }

        // 审查完成，所有风险点已处理
        return (
            <div className="flex flex-col items-center justify-center p-12 text-center">
                <div className="text-6xl mb-4">✅</div>
                <div className="text-xl font-medium text-gray-800 mb-2">
                    所有风险点已处理
                </div>
                <div className="text-sm text-gray-500">共修订{replacedNum}个风险点，合同审查完成</div>
            </div>
        );
    }

    return (
        <>
            <Modal
                title="无法定位到该内容"
                open={isModalVisible}
                onCancel={handleCloseModal}
                footer={[
                    <Button key="cancel" onClick={handleCloseModal}>
                        取消
                    </Button>,
                    <Button key="copy" type="primary" onClick={handleCopyText}>
                        复制原文
                    </Button>,
                ]}
                width={600}
            >
                <div className="py-4">
                    <p className="text-gray-600 mb-3">请手动在文档中查找以下内容：</p>
                    <div className="bg-gray-50 p-4 rounded border border-gray-200 max-h-96 overflow-y-auto">
            <pre className="whitespace-pre-wrap break-words text-sm">
              {modalContent}
            </pre>
                    </div>
                </div>
            </Modal>

            <div className="flex flex-col gap-3">
                <div
                    className="mx-auto w-[31.63rem] flex items-center justify-between "
                >
                    <div className="flex items-center gap-2 px-[1rem] py-[0.38rem] bg-[#F2F8FF] rounded-[0.19rem] border border-[#2260F2]">

                        {isStreaming ? (
                                <>
                                    <span className="inline-block w-2 h-2 bg-[#2260F2] rounded-full animate-pulse" />
                                    <span className="text-[0.75rem] text-[#2260F2]">
                                        {currentProgress?.message || `正在审查中...已审查${riskDataList?.length || 0}个风险点`}
                                    </span>
                                </>
                        ) : (
                            <>
                                <Image src={assets.RisksIcon} alt='risk'/>
                                <span className="text-[0.75rem] text-[#2260F2]">审查完成，共{riskDataList.length}个风险点，已修订{replacedNum}个</span>
                            </>
                        )}
                    </div>

                    <div className="flex items-center gap-2">
                        <span className="text-[0.88rem] text-[#333]">修订状态：</span>
                        <span className={`text-[0.88rem] ${revise ? "text-[#2766FF]" : "text-[#8a8a8a]"}`}>
                        {revise ? "已完成" : "未完成"}</span>
                        <Switch
                            size="small"
                            checked={revise}
                            onChange={(checked) => {
                                if (checked) {
                                    setIsReviseModalOpen(true);
                                } else {
                                    setRevise(false);
                                }
                            }}
                        />
                    </div>
                </div>

                {hasReviewActivity && (
                    <div className="mx-auto w-[31.63rem]">
                        <ReviewProgressPanel
                            currentProgress={currentProgress}
                            progressEvents={progressEvents}
                        />
                    </div>
                )}

                <div className="mx-auto w-[31.63rem] bg-white border border-[#e8edf7] rounded-[0.31rem] p-3">
                    <div className="flex h-2 overflow-hidden rounded bg-[#eef2f7]">
                        <div
                            className="bg-[#ff4d4f]"
                            style={{width: `${(riskOverview.levelCount.high / totalRiskCount) * 100}%`}}
                        />
                        <div
                            className="bg-[#faad14]"
                            style={{width: `${(riskOverview.levelCount.medium / totalRiskCount) * 100}%`}}
                        />
                        <div
                            className="bg-[#52c41a]"
                            style={{width: `${(riskOverview.levelCount.low / totalRiskCount) * 100}%`}}
                        />
                    </div>
                    <div className="mt-2 flex items-center justify-between text-[0.75rem] text-[#555]">
                        <span>高 {riskOverview.levelCount.high}</span>
                        <span>中 {riskOverview.levelCount.medium}</span>
                        <span>低 {riskOverview.levelCount.low}</span>
                    </div>
                    {riskOverview.topTypes.length > 0 && (
                        <div className="mt-3 flex flex-wrap gap-2 text-[0.75rem]">
                            {riskOverview.topTypes.map(([type, count]) => (
                                <span key={type} className="rounded border border-[#dbe6ff] bg-[#f6f9ff] px-2 py-1 text-[#2260F2]">
                                    {type} {count}
                                </span>
                            ))}
                        </div>
                    )}
                </div>

                {sortedRiskData.map((item: RiskResponse) => (
                    <div
                        key={item.id}
                        onClick={() => {
                            setSelectedId(selectedId === item.id ? null : item.id);
                            handleLocate(item.original_content);
                        }}
                        className={`flex flex-col border-2 border-solid rounded-lg bg-white cursor-pointer mx-auto  ${
                            selectedId === item.id ? "border-[#3465eb]" : "border-transparent"
                        }`}
                        style={{width: "31.63rem"}}
                    >
                        {/* 头部：风险点 + 风险等级 + 按钮 */}
                        <div className="m-2 flex items-center mt-[0.94rem]">
                            <span
                                className="border-l-[0.31rem] border-[#3455eb] pl-[0.69rem] text-[1.25rem] text-[black] font-bold mr-[0.94rem] whitespace-nowrap">
                                风险点{item.index}
                            </span>

                            <div className="flex items-center ml-2 space-x-1 whitespace-nowrap flex-shrink-0">
                                <span className="text-[#4D4D4D] text-[0.88rem]">
                                    风险等级：
                                </span>
                                <span className="text-[#ff0000] text-[0.88rem]">
                                    {item.risk_level}
                                </span>
                            </div>
                            {item.risk_type && (
                                <span className="ml-3 rounded border border-[#dbe6ff] bg-[#f6f9ff] px-2 py-[0.13rem] text-[0.75rem] text-[#2260F2]">
                                    {item.risk_type}
                                </span>
                            )}

                            <div
                                className="flex gap-2 ml-auto"
                                onClick={(e) => e.stopPropagation()}
                            >
                                <Button
                                    className="!w-[3.69rem] !h-[1.63rem] !bg-white !text-[#4d4d4d]"
                                    onClick={() => handleIgnore(item.id)}
                                >
                                    忽略
                                </Button>
                                <Button
                                    onClick={() =>
                                        handleReplace(
                                            item.id,
                                            item.original_content,
                                            item.suggested_content
                                        )
                                    }
                                    className="!w-[3.69rem] !h-[1.63rem] !bg-[#2260f2] !text-white"
                                >
                                    修订
                                </Button>
                            </div>
                        </div>

                        {/* 内容区域保持不变 */}
                        <div className="m-2 pl-4 text-[#383838]">
                            <div className="flex items-center gap-2">
                                <span className="text-[1rem] font-medium">原文：</span>
                            </div>
                            <div>{extractTextFromHTML(item.original_content)}</div>
                            <div className="border-b-[#fafafa] mb-[0.31rem] border-b-[0.1rem] pb-[0.3rem]">
                                <span className="text-[1rem] font-medium">风险分析：</span>
                                <div>{item.risk_analysis}</div>
                            </div>
                            <div>
                                <span className="text-[1rem] font-medium">修订建议：</span>
                                <div className="text-[#2260F2]">
                                    {extractTextFromHTML(item.suggested_content) || (isStreaming ? "修改建议生成中..." : "暂无自动建议，请人工复核")}
                                </div>
                            </div>
                            {item.reason && (
                                <div className="mt-2">
                                    <span className="text-[1rem] font-medium">修改理由：</span>
                                    <div>{item.reason}</div>
                                </div>
                            )}
                        </div>
                    </div>
                ))}
                <div style={{height: "6rem", flexShrink: 0, width: "100%"}}/>
            </div>
            <Modal
                open={isReviseModalOpen}
                onOk={handleReviseOk}
                onCancel={handleReviseCancel}
                okText="确定"
                cancelText="取消"
                title='确认标记此合同为“已修订”吗？'
            >
                <p>确认后，该合同的状态将更新为“已修订”。</p>
            </Modal>

            {/* 审查完成提示 Modal */}
            <Modal
                open={isCompletedModalOpen}
                onOk={() => setIsCompletedModalOpen(false)}
                onCancel={() => setIsCompletedModalOpen(false)}
                okText="知道了"
                cancelButtonProps={{ style: { display: 'none' } }}
                centered
                title={null}
                closable={false}
            >
                <div className="flex flex-col items-center py-6">
                    <div className="text-5xl mb-4">✅</div>
                    <div className="text-xl font-medium text-gray-800 mb-2">
                        审查完成
                    </div>
                    <div className="text-sm text-gray-500">
                        共发现 {riskDataList.length} 个风险点，请及时处理
                    </div>
                </div>
            </Modal>
        </>
    );
}

// ============ 辅助函数 ============

/**
 * 尝试使用编辑器 API 定位
 */
async function tryEditorAPI(
    editor: InstanceType<typeof Editor>,
    searchText: string
): Promise<boolean> {
    const command = editor.command;

    const strategies = buildEditorSearchTexts(searchText, true);

    for (const text of strategies) {
        if (!text) continue;

        command.executeSearch(text);

        await new Promise((resolve) => {
            requestAnimationFrame(() => {
                requestAnimationFrame(() => {
                    requestAnimationFrame(resolve);
                });
            });
        });

        const navigateInfo = command.getSearchNavigateInfo?.();
        if (navigateInfo && navigateInfo.count > 0) {
            command.executeSearchNavigateNext?.();
            await new Promise((resolve) =>
                requestAnimationFrame(() => requestAnimationFrame(resolve))
            );

            const keywordContext = command.getKeywordContext?.(text);
            if (keywordContext && keywordContext.length > 0) {
                const rangeList = keywordContext.map((ctx) => ctx.range);
                const firstRange = rangeList[0];

                if (firstRange.tableId) {
                    command.executeSetPositionContext?.(firstRange);
                }

                command.executeSetRange(
                    firstRange.startIndex,
                    firstRange.endIndex,
                    firstRange.tableId,
                    firstRange.startTdIndex,
                    firstRange.endTdIndex,
                    firstRange.startTrIndex,
                    firstRange.endTrIndex
                );

                await new Promise((resolve) => requestAnimationFrame(resolve));

                if (command.executeFocus) {
                    command.executeFocus({
                        range: firstRange,
                        isMoveCursorToVisible: true,
                    });
                }

                command.executeSearch(null);
                return true;
            }
        }

        command.executeSearch(null);
    }

    return false;
}

/**
 * 尝试使用字符映射器定位
 */
function tryCharacterMapper(
    mapper: CharacterMapper,
    searchText: string
): boolean {
    const result = mapper.smartFind(searchText);

    if (result) {
        const success = mapper.highlightRange(result.start, result.end);
        return success;
    }

    return false;
}

/**
 * 尝试使用字符映射器在 DOM 中替换原文。
 * docx 导入会把文档渲染成 contenteditable HTML（编辑器 elementList 为空），
 * 因此修订必须直接操作 DOM：定位原文范围 -> 删除 -> 插入修订建议。
 */
function tryCharacterMapperReplace(
    mapper: CharacterMapper,
    searchText: string,
    replaceText: string
): boolean {
    const match = mapper.smartFind(searchText);
    if (!match) return false;

    const textRange = mapper.getRange(match.start, match.end);
    if (!textRange) return false;

    try {
        const range = document.createRange();
        range.setStart(textRange.startNode, textRange.startOffset);
        range.setEnd(textRange.endNode, textRange.endOffset);

        // 删除原文本，插入修订建议（保留为纯文本，避免 HTML 注入）
        range.deleteContents();
        const textNode = document.createTextNode(replaceText);
        range.insertNode(textNode);

        // 选中新插入的文本，方便用户查看或继续编辑
        const selection = window.getSelection();
        if (selection) {
            const nextRange = document.createRange();
            nextRange.setStart(textNode, 0);
            nextRange.setEnd(textNode, replaceText.length);
            selection.removeAllRanges();
            selection.addRange(nextRange);
        }

        // 替换后重建映射
        setTimeout(() => mapper.rebuild?.(), 300);
        return true;
    } catch (e) {
        console.warn("字符映射替换失败:", e);
        return false;
    }
}

/**
 * 尝试使用传统 DOM 搜索定位
 */
async function tryDOMSearch(searchText: string): Promise<boolean> {
    const canvasContainer = document.querySelector('[id^="canvasEditor-"]');
    if (!canvasContainer) return false;

    const fullText = canvasContainer.textContent || "";

    const createNormalizer = (level: number) => {
        return (text: string) => {
            switch (level) {
                case 0:
                    return text;
                case 1:
                    return text.replace(/\s+/g, " ").trim();
                case 2:
                    return text
                        .replace(/[\r\n]+/g, "")
                        .replace(/\s+/g, " ")
                        .trim();
                case 3:
                    return text.replace(/\s+/g, "");
                default:
                    return text;
            }
        };
    };

    for (let level = 0; level <= 3; level++) {
        const normalize = createNormalizer(level);
        const normalizedFullText = normalize(fullText);
        const normalizedSearchText = normalize(searchText);

        if (normalizedSearchText.length < 5) continue;

        const foundIndex = normalizedFullText.indexOf(normalizedSearchText);
        if (foundIndex === -1) continue;

        const walker = document.createTreeWalker(
            canvasContainer,
            NodeFilter.SHOW_TEXT,
            null
        );

        let currentPos = 0;
        let startNode: Node | null = null;
        let startOffset = 0;
        let endNode: Node | null = null;
        let endOffset = 0;
        let foundStart = false;

        let node;
        while ((node = walker.nextNode())) {
            const nodeText = node.textContent || "";
            const normalizedNodeText = normalize(nodeText);
            const nodeStart = currentPos;
            const nodeEnd = currentPos + normalizedNodeText.length;

            if (!foundStart && foundIndex >= nodeStart && foundIndex < nodeEnd) {
                startNode = node;
                startOffset = foundIndex - nodeStart;
                foundStart = true;
            }

            if (
                foundStart &&
                foundIndex + normalizedSearchText.length > nodeStart &&
                foundIndex + normalizedSearchText.length <= nodeEnd
            ) {
                endNode = node;
                endOffset = foundIndex + normalizedSearchText.length - nodeStart;

                if (startNode && endNode) {
                    try {
                        const scrollElement =
                            startNode.parentElement || endNode.parentElement;
                        if (scrollElement) {
                            scrollElement.scrollIntoView({
                                behavior: "smooth",
                                block: "center",
                            });
                        }

                        const range = document.createRange();
                        const safeStartOffset = Math.min(
                            Math.max(0, startOffset),
                            startNode.textContent?.length || 0
                        );
                        const safeEndOffset = Math.min(
                            Math.max(0, endOffset),
                            endNode.textContent?.length || 0
                        );

                        range.setStart(startNode, safeStartOffset);
                        range.setEnd(endNode, safeEndOffset);

                        const selection = window.getSelection();
                        if (selection) {
                            selection.removeAllRanges();
                            selection.addRange(range);
                        }

                        return true;
                    } catch (e) {
                        console.warn("选区创建失败:", e);
                    }
                }
                return false;
            }

            currentPos = nodeEnd;
        }
    }

    return false;
}

/**
 * 尝试使用编辑器 API 替换
 */
async function tryEditorAPIReplace(
    editor: InstanceType<typeof Editor>,
    searchText: string,
    replaceText: string
): Promise<boolean> {
    const command = editor.command;

    const strategies = buildEditorSearchTexts(searchText, false);

    for (const text of strategies) {
        if (!text) continue;

        command.executeSearch(text);

        await new Promise((resolve) => {
            requestAnimationFrame(() => {
                requestAnimationFrame(() => {
                    requestAnimationFrame(resolve);
                });
            });
        });

        const navigateInfo = command.getSearchNavigateInfo?.();
        if (navigateInfo && navigateInfo.count > 0) {
            command.executeSearchNavigateNext?.();
            await new Promise((resolve) =>
                requestAnimationFrame(() => requestAnimationFrame(resolve))
            );

            const keywordContext = command.getKeywordContext?.(text);
            if (keywordContext && keywordContext.length > 0) {
                const rangeList = keywordContext.map((ctx) => ctx.range);
                const firstRange = rangeList[0];

                if (firstRange.tableId) {
                    command.executeSetPositionContext?.(firstRange);
                }

                command.executeSetRange(
                    firstRange.startIndex,
                    firstRange.endIndex,
                    firstRange.tableId,
                    firstRange.startTdIndex,
                    firstRange.endTdIndex,
                    firstRange.startTrIndex,
                    firstRange.endTrIndex
                );

                await new Promise((resolve) => requestAnimationFrame(resolve));

                let baseFormat: Partial<IElement> = {};
                try {
                    const rangeContext = command.getRangeContext?.();
                    if (
                        rangeContext &&
                        rangeContext.selectionElementList &&
                        rangeContext.selectionElementList.length > 0
                    ) {
                        baseFormat = pickBaseFormat(rangeContext.selectionElementList);
                    }
                } catch (error) {
                    console.warn("获取原文格式失败，使用默认格式:", error);
                }

                // 插入带红色标记的文本，保留原格式
                const suggestedElements = replaceText.split("").map((char) => ({
                    ...baseFormat, // 保留原格式
                    value: char,
                    color: "#FF0000",
                }));
                command.executeInsertElementList(suggestedElements);

                command.executeSearch(null);
                return true;
            }
        }

        command.executeSearch(null);
    }

    return false;
}
