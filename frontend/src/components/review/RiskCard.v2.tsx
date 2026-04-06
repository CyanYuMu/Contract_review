import React, {useEffect, useRef, useState} from "react";
import {Button, Modal, Switch} from "antd";
import toast from "react-hot-toast";
import {RiskResponse} from "@/lib/Interface";
import Editor from "@/lib/canvas-editor/editor";
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
        if (!editor) return;

        try {
            // 先提取 HTML 标签中的纯文本
            const plainText = extractTextFromHTML(originalContent);
            const cleanedOriginal = removeQuotes(plainText.trim());

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
        } catch (error) {
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
        if (!editor) return;

        try {
            // 先提取 HTML 标签中的纯文本
            const plainText = extractTextFromHTML(originalContent);
            const cleanedOriginal = removeQuotes(plainText.trim());

            // 优先级1: 优先使用编辑器 API
            const editorResult = await tryEditorAPIReplace(
                editor,
                cleanedOriginal,
                suggestedContent
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

            // 优先级2: 使用字符映射器
            if (mapperRef.current) {
                const mapperResult = tryCharacterMapperReplace(
                    mapperRef.current,
                    cleanedOriginal,
                    suggestedContent
                );
                if (mapperResult) {
                    // 修订完成后，删除这条风险数据
                    removeRiskData(riskId);
                    // 已修订数量加1
                    addReplacedNum()
                    toast.success("修订成功");

                    // 替换后重建映射
                    setTimeout(() => {
                        mapperRef.current?.rebuild();
                    }, 300);
                    return;
                }
            }

            showModal(cleanedOriginal);
        } catch (error) {
            toast.error("替换操作失败，请稍后重试");
        }
    };

    // 空列表处理：区分"审查中"和"已全部修订"
    if (riskDataList.length === 0) {
        // 正在审查中（还没有数据）
        if (isStreaming || !isCompleted) {
            return (
                <div className="reviewing-wrapper">
                    <div className="reviewing-container">
                        <div className="reviewing-spinner"></div>
                        <div className="reviewing-text">审查中</div>
                    </div>
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

    const sortedRiskData = [...riskDataList].sort((a, b) => a.index - b.index);

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
                                    <span className="text-[0.75rem] text-[#2260F2]">正在审查中...已审查{riskDataList?.length||0}个风险点</span>
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
                                    {extractTextFromHTML(item.suggested_content)}
                                </div>
                            </div>
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

    const strategies = [() => searchText, () => searchText.replace(/\s+/g, " ")];

    for (const strategy of strategies) {
        const text = strategy();
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

    const strategies = [() => searchText, () => searchText.replace(/\s+/g, " ")];

    for (const strategy of strategies) {
        const text = strategy();
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

                // 尝试获取原文格式
                // eslint-disable-next-line
                let baseFormat: any = {};
                try {
                    const rangeContext = command.getRangeContext?.();
                    if (
                        rangeContext &&
                        rangeContext.selectionElementList &&
                        rangeContext.selectionElementList.length > 0
                    ) {
                        // 使用第一个字符的格式作为基准
                        const firstElement = rangeContext.selectionElementList[0];
                        baseFormat = {
                            font: firstElement.font,
                            size: firstElement.size,
                            bold: firstElement.bold,
                            italic: firstElement.italic,
                            underline: firstElement.underline,
                            strikeout: firstElement.strikeout,
                        };
                    }
                } catch (error) {
                    console.warn("获取原文格式失败，使用默认格式:", error);
                }

                // 插入带红色标记的文本，保留原格式
                const suggestedElements = replaceText.split("").map((char) => ({
                    value: char,
                    color: "#FF0000",
                    ...baseFormat, // 保留原格式
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

/**
 * 尝试使用字符映射器替换
 */
function tryCharacterMapperReplace(
    mapper: CharacterMapper,
    searchText: string,
    replaceText: string
): boolean {
    const result = mapper.smartFind(searchText);

    if (result) {
        const textRange = mapper.getRange(result.start, result.end);
        if (!textRange) return false;

        try {
            const range = document.createRange();
            range.setStart(textRange.startNode, textRange.startOffset);
            range.setEnd(textRange.endNode, textRange.endOffset);

            // 尝试获取原文的样式
            const parentElement = textRange.startNode.parentElement;
            let computedStyle: CSSStyleDeclaration | null = null;

            if (parentElement) {
                try {
                    computedStyle = window.getComputedStyle(parentElement);
                } catch (error) {
                    console.warn("获取原文样式失败:", error);
                }
            }

            range.deleteContents();

            // 创建带红色样式的 span 元素，保留原格式
            const span = document.createElement("span");
            span.style.color = "#FF0000";

            // 尝试复制原文的样式（除了颜色）
            if (computedStyle) {
                try {
                    // 复制字体相关样式
                    if (computedStyle.fontFamily) {
                        span.style.fontFamily = computedStyle.fontFamily;
                    }
                    if (computedStyle.fontSize) {
                        span.style.fontSize = computedStyle.fontSize;
                    }
                    if (computedStyle.fontWeight && computedStyle.fontWeight !== "400") {
                        span.style.fontWeight = computedStyle.fontWeight;
                    }
                    if (computedStyle.fontStyle && computedStyle.fontStyle !== "normal") {
                        span.style.fontStyle = computedStyle.fontStyle;
                    }
                    if (
                        computedStyle.textDecoration &&
                        computedStyle.textDecoration !== "none"
                    ) {
                        // 保留下划线等装饰，但不包括颜色
                        const decorations = computedStyle.textDecoration.split(" ");
                        const decorationLine = decorations.find(
                            (d) =>
                                d === "underline" || d === "overline" || d === "line-through"
                        );
                        if (decorationLine) {
                            span.style.textDecoration = decorationLine;
                        }
                    }
                    if (
                        computedStyle.letterSpacing &&
                        computedStyle.letterSpacing !== "normal"
                    ) {
                        span.style.letterSpacing = computedStyle.letterSpacing;
                    }
                    if (
                        computedStyle.backgroundColor &&
                        computedStyle.backgroundColor !== "rgba(0, 0, 0, 0)" &&
                        computedStyle.backgroundColor !== "transparent"
                    ) {
                        span.style.backgroundColor = computedStyle.backgroundColor;
                    }
                } catch (error) {
                    console.warn("复制样式失败:", error);
                }
            }

            span.textContent = replaceText;
            range.insertNode(span);

            range.setStartAfter(span);
            range.collapse(true);

            const selection = window.getSelection();
            if (selection) {
                selection.removeAllRanges();
                selection.addRange(range);
            }

            // 滚动到视图
            span.scrollIntoView({
                behavior: "smooth",
                block: "center",
            });

            return true;
        } catch (e) {
            console.error("替换失败:", e);
            return false;
        }
    }

    return false;
}
