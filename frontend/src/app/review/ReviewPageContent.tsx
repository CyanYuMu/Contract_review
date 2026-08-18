"use client";

import React, {useEffect, useMemo, useRef, useState,useCallback,} from "react";
import "./page.css";
import Editor, {
  EditorMode,
  type IEditorData,
  type IEditorOption,
  type IElement,
  PageMode,
} from "@/lib/canvas-editor/editor";
import {type EditorState as EditorStateType, useEditorListeners,} from "@/hooks/useEditorListeners";
import {markdownPlugin} from "@/lib/canvas-editor/plugins/markdown";
import RiskCard from "@/components/review/RiskCard.v2";
import Topbar from "@/components/Topbar";
import EditorToolbar from "@/components/editor/EditorToolbar";
import {UploadStore, type UploadData} from "@/store/uploadStore";
import {RiskStore} from "@/store/riskStore";
import ReviewHistory from "@/components/list/ReviewHistory";
import ContrastHistory from "@/components/list/ContrastHistory";
import type {TabType} from "@/components/TopbarTabs";
import type {User,ApiError} from "@/lib/Interface";
import {getUserInfo} from "@/lib/api/user";
import LoginModal from "@/components/auth/LoginModal";
import {Button, Space} from "antd";
import {useRouter} from "next/navigation";
import {save as saveApi} from "@/lib/api/upload";
import {resolveFileUrl} from "@/utils/url";
import ContractContrastPanel from "@/components/contrast/ContractContrastPanel";
import QAPanel from "@/components/qa/QAPanel";
import {authDatedHandler} from "@/utils/authDatedHandler";
import { buildStaticFileUrl } from '@/utils/url';
import {waitFor} from "@/utils/waitFor";
import {getAuthToken} from "@/utils/client";

function readStoredReviewUploadData(): UploadData | null {
    if (typeof window === "undefined") return null;

    const storedFileUrl = localStorage.getItem("uploaded_file_url") || undefined;
    const storedFileType = localStorage.getItem("uploaded_file_type") || undefined;
    const storedFileTitle = localStorage.getItem("uploaded_file_title") || undefined;
    const storedPartyA = localStorage.getItem("uploaded_party_a") || undefined;
    const storedPartyB = localStorage.getItem("uploaded_party_b") || undefined;
    const storedFileId = Number(localStorage.getItem("uploaded_file_id") || 0);
    const storedContractTypeId = Number(localStorage.getItem("uploaded_contract_type_id") || 0);

    if (!storedFileUrl && !storedFileType && !storedFileTitle) {
        return null;
    }

    return {
        file_url: storedFileUrl,
        file_type: storedFileType,
        title: storedFileTitle,
        party_a: storedPartyA,
        party_b: storedPartyB,
        file_id: Number.isFinite(storedFileId) && storedFileId > 0 ? storedFileId : undefined,
        contract_type_id:
            Number.isFinite(storedContractTypeId) && storedContractTypeId > 0
                ? storedContractTypeId
                : undefined,
    };
}

function persistReviewUploadData(uploadData: UploadData) {
    if (typeof window === "undefined" || !uploadData?.file_url) return;

    localStorage.setItem("uploaded_file_url", uploadData.file_url);
    if (uploadData.file_type) localStorage.setItem("uploaded_file_type", uploadData.file_type);
    if (uploadData.title) localStorage.setItem("uploaded_file_title", uploadData.title);
    if (uploadData.file_id) localStorage.setItem("uploaded_file_id", String(uploadData.file_id));
    if (uploadData.contract_type_id) {
        localStorage.setItem("uploaded_contract_type_id", String(uploadData.contract_type_id));
    }
    if (uploadData.party_a !== undefined && uploadData.party_a !== null) {
        localStorage.setItem("uploaded_party_a", uploadData.party_a);
    }
    if (uploadData.party_b !== undefined && uploadData.party_b !== null) {
        localStorage.setItem("uploaded_party_b", uploadData.party_b);
    }
}

export default function ReviewPageContent() {
    const [activeTab, setActiveTab] = useState<TabType>("check");
    const [user, setUser] = useState<User | null>(null);
    const [loginVisible, setLoginVisible] = useState(false);
    const [editorKey, setEditorKey] = useState(0);
    const [shouldRenderEditor, setShouldRenderEditor] = useState(false);
    const [isLoadingDocument, setIsLoadingDocument] = useState(false);
    const [documentError, setDocumentError] = useState<string | null>(null);
    const canvasContainerRef = useRef<HTMLDivElement>(null);
    const canvasEditorRef = useRef<InstanceType<typeof Editor> | null>(null);
    const [editorState, setEditorState] = useState<EditorStateType | null>(null);
    const lastTabRef = useRef<TabType>("check");
    const [historyType, setHistoryType] = useState<"review" | "contrast">("review");
    const riskDataList = RiskStore((e) => e.riskDataList);
    const isStreaming = RiskStore((e) => e.isStreaming);
    const sourceFileUrl = RiskStore((e) => e.sourceFileUrl);
    const resetRiskData = RiskStore((e) => e.resetRiskData);
    const data = UploadStore((e) => e.data);
    const setData = UploadStore((e) => e.setData);
    const [restoredUploadData, setRestoredUploadData] = useState<UploadData | null>(null);
    const router = useRouter();

    const isReviewing = isStreaming;

    const restoreReviewUploadData = useCallback(() => {
        const currentData = UploadStore.getState().data;
        if (currentData?.file_url) {
            persistReviewUploadData(currentData);
            setRestoredUploadData(currentData);
            return currentData;
        }

        const storedData = readStoredReviewUploadData();
        if (!storedData?.file_url) return null;

        const mergedData = {
            ...currentData,
            ...storedData,
        };
        setData(mergedData);
        setRestoredUploadData(mergedData);
        return mergedData;
    }, [setData]);

    useEffect(() => {
        restoreReviewUploadData();
    }, [restoreReviewUploadData]);

    useEffect(() => {
        if (data?.file_url) {
            persistReviewUploadData(data);
            setRestoredUploadData(data);
        }
    }, [data]);

    useEffect(() => {
        if (activeTab === "check" && !data?.file_url) {
            restoreReviewUploadData();
        }
    }, [activeTab, data?.file_url, restoreReviewUploadData]);

    const effectiveData = data?.file_url ? data : restoredUploadData || data;
    const title = effectiveData?.title ?? "";
    const file_type = effectiveData?.file_type ?? "docx";
    const file_url = resolveFileUrl(effectiveData?.file_url);
    const documentKey = useMemo(() => {
        if (!file_url) return "";
        let hash = 0;
        for (let i = 0; i < file_url.length; i++) {
            const char = file_url.charCodeAt(i);
            hash = (hash << 5) - hash + char;
            hash = hash & hash;
        }
        return `doc-${Math.abs(hash)}`;
    }, [file_url]);

    // 当文档 URL 变化时，检查是否需要清除旧的风险点数据
    useEffect(() => {
        const normalizedSourceFileUrl = resolveFileUrl(sourceFileUrl || undefined);
        if (file_url && normalizedSourceFileUrl && file_url !== normalizedSourceFileUrl) {
            // 当前文档和风险点数据不匹配，清除旧数据
            resetRiskData();
        }
    }, [file_url, sourceFileUrl, resetRiskData]);

    // 仅用于控制 Canvas Editor 的挂载时机

    useEffect(() => {
        if (file_url && file_type) {
            setShouldRenderEditor(false);
            setEditorKey((prev) => prev + 1);

            const timer = setTimeout(() => {
                setShouldRenderEditor(true);
            }, 100);

            return () => clearTimeout(timer);
        } else {
            setShouldRenderEditor(false);
        }
    }, [file_url, file_type]);

    useEffect(() => {
        const lastTab = lastTabRef.current;
        if (activeTab === "check" && lastTab !== "check" && file_url && file_type) {
            setShouldRenderEditor(false);
            setEditorKey((prev) => prev + 1);
            const timer = setTimeout(() => {
                setShouldRenderEditor(true);
            }, 80);
            lastTabRef.current = activeTab;
            return () => clearTimeout(timer);
        }
        lastTabRef.current = activeTab;
    }, [activeTab, file_url, file_type]);

    useEffect(() => {
        if (activeTab === "contrast") {
            setHistoryType("contrast");
        } else if (activeTab === "history") {
            setHistoryType("review");
        }
    }, [activeTab]);

    // 加载文档内容
    useEffect(() => {
        if (!file_url || !shouldRenderEditor) return;

        const loadDocument = async () => {
            setIsLoadingDocument(true);
            setDocumentError(null); // 清除之前的错误
            try {
       
                // const url = new URL(file_url);
                // const path = url.pathname + url.search;
                // const proxyPath = path.replace(/^\/api\/static/, '/api/proxy/static');
                // const proxy_url = process.env.NEXT_SERVER_URL + proxyPath;
                // console.log("代理地址为：",proxy_url)
                console.log("原始文件路径：",file_url)
                const proxyUrl = buildStaticFileUrl(file_url); 
                const token = getAuthToken();
                const response = await fetch(proxyUrl, {
                    headers: token ? {Authorization: 'Bearer ' + token} : undefined,
                });
           
                // const response = await fetch(file_url);
                if (!response.ok) {
                    throw new Error(
                        `加载文档失败: ${response.status} ${response.statusText}`
                    );
                }

                // 根据文件类型处理
                let elementList: IElement[] = [];
                const headerElementList: IElement[] = [];
                const footerElementList: IElement[] = [];
                let docxArrayBuffer: ArrayBuffer | null = null; // 用于存储 DOCX 文件的 arrayBuffer
                docxArrayBuffer = await response.arrayBuffer();
                elementList = [];
                if (!elementList || elementList.length === 0) {
                    elementList = [{value: "\n"}];
                }

                // 等待 DOM 容器准备好（在解析完成后再次检查）
                await waitFor(() => canvasContainerRef.current, { timeout: 2000, interval: 100 });

                if (!canvasContainerRef.current) {
                    throw new Error("编辑器容器未准备好");
                }

                // 销毁已有实例
                if (canvasEditorRef.current) {
                    try {
                        canvasEditorRef.current.destroy();
                    } catch (e) {
                    }
                    canvasEditorRef.current = null;
                }

                // 创建新实例
                const editorOptions: IEditorOption = {
                    mode: EditorMode.EDIT,
                    width: 794, // A4 宽度（像素）
                    height: 1123, // A4 高度（像素）
                    scale: 1,
                    pageMode: PageMode.PAGING, // 分页模式，显示纸张布局
                    pageGap: 20, // 页面之间的间隔
                    margins: [100, 120, 100, 120], // 纸张内边距，分别为：上、右、下、左
                    marginIndicatorSize: 35, // 纸张内边距指示器的大小，也就是四个直角的边长
                    marginIndicatorColor: "#BABABA", // 纸张内边距指示器的颜色，也就是四个直角的边颜色
                };

                // 准备编辑器数据，如果有页眉页脚，使用 IEditorData 格式
                let editorData: IEditorData | IElement[];
                if (
                    (headerElementList && headerElementList.length > 0) ||
                    (footerElementList && footerElementList.length > 0)
                ) {
                    editorData = {
                        main: elementList,
                        header:
                            headerElementList && headerElementList.length > 0
                                ? headerElementList
                                : undefined,
                        footer:
                            footerElementList && footerElementList.length > 0
                                ? footerElementList
                                : undefined,
                    };
                    // 启用页眉页脚
                    editorOptions.header = {
                        disabled: false,
                    };
                    editorOptions.footer = {
                        disabled: false,
                    };
                } else {
                    editorData = elementList;
                }

                if (docxArrayBuffer) {
                    if (typeof window !== "undefined") {
                        try {
                            await Promise.all([
                                import("@/lib/canvas-editor/plugins/docx/importDocx"),
                                import("@/lib/canvas-editor/plugins/docx/exportDocx"),
                            ]);

                            const docxPluginModule = await import(
                                "@/lib/canvas-editor/plugins/docx"
                                );
                            const docxPlugin = docxPluginModule.default;

                            canvasEditorRef.current = new Editor(
                                canvasContainerRef.current,
                                editorData,
                                editorOptions
                            );

                            canvasEditorRef.current.use(markdownPlugin);
                            canvasEditorRef.current.use(docxPlugin);

                            await waitFor(
                                () => !!canvasEditorRef.current?.command?.executeImportDocx,
                                { timeout: 2500, interval: 50 }
                            );

                            if (canvasEditorRef.current.command.executeImportDocx) {
                                await canvasEditorRef.current.command.executeImportDocx({
                                    arrayBuffer: docxArrayBuffer,
                                });
                            } else {
                                throw new Error("DOCX 插件加载超时");
                            }
                        } catch (err) {
                            console.error("[文档加载] DOCX 加载失败:", err);
                            canvasEditorRef.current = new Editor(
                                canvasContainerRef.current,
                                editorData,
                                editorOptions
                            );
                            canvasEditorRef.current.use(markdownPlugin);
                            setDocumentError("DOCX 文件加载失败，请重试");
                        }
                    } else {
                        canvasEditorRef.current = new Editor(
                            canvasContainerRef.current,
                            editorData,
                            editorOptions
                        );
                        canvasEditorRef.current.use(markdownPlugin);
                    }
                } else {
                    canvasEditorRef.current = new Editor(
                        canvasContainerRef.current,
                        editorData,
                        editorOptions
                    );

                    if (canvasEditorRef.current) {
                        canvasEditorRef.current.use(markdownPlugin);

                        if (typeof window !== "undefined") {
                            try {
                                const docxPluginModule = await import(
                                    "@/lib/canvas-editor/plugins/docx"
                                    );
                                const docxPlugin = docxPluginModule.default;
                                canvasEditorRef.current.use(docxPlugin);
                            } catch (err) {
                                console.warn(
                                    "[文档加载] DOCX 插件加载失败（导出功能可能不可用）:",
                                    err
                                );
                            }
                        }
                    }
                }
                if (canvasContainerRef.current) {
                    const editorContainer =
                        canvasContainerRef.current.querySelector(
                            ".ce-page-container"
                        )?.parentElement;
                    if (editorContainer) {
                        editorContainer.style.margin = "0 auto";
                        editorContainer.style.display = "flex";
                        editorContainer.style.justifyContent = "center";
                    }
                }
            } catch (error) {
                if (error instanceof Error) {
                    setDocumentError(error.message);
                } else {
                    setDocumentError("加载文档时发生未知错误");
                }
            } finally {
                setIsLoadingDocument(false);
            }
        };

        // 使用 setTimeout 确保 DOM 已渲染
        const timer = setTimeout(() => {
            loadDocument();
        }, 100);

        return () => {
            clearTimeout(timer);
            if (canvasEditorRef.current) {
                try {
                    canvasEditorRef.current.destroy();
                } catch {
                }
                canvasEditorRef.current = null;
            }
        };
    }, [file_url, file_type, shouldRenderEditor, editorKey]);

    useEffect(() => {
        const checkLoginStatus = async () => {
            const token = localStorage.getItem("access_token");
            if (token) {
                try {
                    const userInfo = await getUserInfo();
                    setUser(userInfo);
                } catch (error) {
                    localStorage.removeItem("access_token");
                    const apiError = error as ApiError;
                    if (apiError.status === 401) {
                        setLoginVisible(true);
                    }
                }
            }
        };
        checkLoginStatus();
    }, []);

    // 接入编辑器监听：当编辑器实例可用时，实时同步状态到本地 editorState
    useEditorListeners({
        editor: canvasEditorRef.current,
        onStateChange: (updates) => {
            setEditorState((prev) => {
                return {
                    ...(prev || ({} as EditorStateType)),
                    ...updates,
                } as EditorStateType;
            });
        },
    });

    const handleLoginSuccess = async (token: string) => {
        try {
            const userInfo = await getUserInfo();
            setUser(userInfo);
            if (token) {
                localStorage.setItem("access_token", token);
            }
        } catch (error) {
            const apiError = error as ApiError;
            if (apiError.status === 401) {
                setLoginVisible(true);
            }
        }
        setLoginVisible(false);
        // 登录成功后刷新页面
        window.location.reload();
    };

    const handleLoginClick = useCallback(() => {
        setLoginVisible(true);
    }, []);

    // 注册登录回调，使 403/401 模态框可以触发登录模态框
    useEffect(() => {
        const unregister = authDatedHandler.registerLoginCallback(() => {
            handleLoginClick();
        });
        return () => {
            unregister();
        };
    }, [handleLoginClick]);

    const handleSaveDocument = async () => {
        if (!canvasEditorRef.current?.command) {
            return;
        }

        // 等待 executeExportDocx 方法加载
        await waitFor(
            () => !!canvasEditorRef.current?.command?.executeExportDocx,
            { timeout: 5000, interval: 50 }
        );

        if (!canvasEditorRef.current.command.executeExportDocx) {
            // 尝试手动加载 docx 插件
            try {
                const docxPluginModule = await import(
                    "@/lib/canvas-editor/plugins/docx"
                    );
                const docxPlugin = docxPluginModule.default;
                canvasEditorRef.current.use(docxPlugin);

                // 再次等待
                await waitFor(
                    () => !!canvasEditorRef.current?.command?.executeExportDocx,
                    { timeout: 5000, interval: 50 }
                );

                if (!canvasEditorRef.current.command.executeExportDocx) {
                    return;
                }
            } catch (err) {
                return;
            }
        }

        try {
            const fileName = title || "合同文档";
            const file = await canvasEditorRef.current.command.executeExportDocx({
                fileName: fileName.replaceAll(".docx", "").replaceAll(".doc", ""),
            });
            const fd = new FormData();
            fd.append("file", file);
            await saveApi(fd);
        } catch (error) {
            console.error("导出失败:", error);
        }
    };

    // 重新上传：清空当前审阅状态并回到主页（上传入口）
    const handleReUpload = () => {
        resetRiskData();
        if (typeof window !== "undefined") {
            // 清除审阅工作区与上传缓存，保证主页呈现全新上传界面
            ["review_workspace_active", "review_session_id", "uploaded_file_url",
                "uploaded_file_id", "uploaded_file_title", "uploaded_party_a",
                "uploaded_party_b", "uploaded_file_type", "uploaded_contract_type_id",
            ].forEach((k) => window.localStorage.removeItem(k));
        }
        router.push("/");
    };

    const renderSideContent = () => {
        switch (activeTab) {
            case "check":
                return (
                    <RiskCard
                        riskDataList={riskDataList}
                        editor={canvasEditorRef.current}
                    />
                );
            case "contrast":
                return (
                    <div className="p-4 bg-gray-100 h-full text-gray-600">
                        合同比对面板（待接入）
                    </div>
                );
            default:
                return (
                    <RiskCard
                        riskDataList={riskDataList}
                        editor={canvasEditorRef.current}
                    />
                );
        }
    };

    return (
        <div className="flex flex-col h-screen overflow-hidden">
            <Topbar
                user={user}
                onLoginClick={handleLoginClick}
                onLogoutClick={() => {
                    localStorage.removeItem("access_token");
                    localStorage.removeItem("refresh_token");
                    localStorage.removeItem("token_type");
                    setUser(null);
                }}
                activeTab={activeTab}
                onTabClick={setActiveTab}
            />
            <div
                className="flex flex-row flex-1"
                style={{paddingRight: activeTab === 'check' ? "35.13rem" : 0, overflow: "hidden"}}
            >
                {activeTab === 'history' ? (
                    <div className="flex-1 w-full overflow-auto bg-[#f3f4f6]">
                        <div className="h-full rounded-[0.31rem] border border-[#e3e3e3] shadow-sm p-6 overflow-auto">
                            {historyType === "review" ? (
                                <ReviewHistory
                                    type="Review"
                                    onTypeChange={(type) => setHistoryType(type === "Review" ? "review" : "contrast")}
                                    onViewRecord={() => setActiveTab("check")}
                                />
                            ) : (
                                <ContrastHistory
                                    type="Contrast"
                                    onTypeChange={(type) => setHistoryType(type === "Contrast" ? "contrast" : "review")}
                                />
                            )}
                        </div>
                    </div>
                ) : activeTab === 'qa' ? (
                    <div className="flex-1 overflow-hidden bg-white">
                        <QAPanel/>
                    </div>
                ) : activeTab === 'contrast' ? (
                    <div className="flex-1 overflow-hidden">
                        <div className="h-full mt-[2.75rem] overflow-hidden">
                            <ContractContrastPanel/>
                        </div>
                    </div>
                ) : (
                    <div
                        className="flex-1 editor-area"
                        style={{
                            display: "flex",
                            flexDirection: "column",
                            overflow: "hidden",
                        }}
                    >
                        <div
                            className="flex items-center justify-between gap-3 px-4 py-2 bg-white border-b border-[#e3e3e3]"
                            style={{zIndex: 10000, pointerEvents: isReviewing ? 'none' : 'auto'}}
                        >
                            <div className="flex items-center gap-2 min-w-0">
                                <span className="text-[0.88rem] text-[#333] truncate max-w-[22rem]" title={title}>
                                    {title || "合同文档"}
                                </span>
                            </div>
                            <Button size="small" onClick={handleReUpload}>
                                重新上传
                            </Button>
                        </div>
                        <div style={{zIndex: 10000, pointerEvents: isReviewing ? 'none' : 'auto'}}>
                            <EditorToolbar
                                editor={canvasEditorRef.current}
                                onSave={handleSaveDocument}
                            />
                        </div>
                        <div
                            className="flex-1 w-full relative word-scroll-area"
                            style={{overflow: "hidden", minHeight: 0, position: "relative"}}
                        >
                            {file_url && file_type && shouldRenderEditor ? (
                                <>
                                    <div
                                        key={`${documentKey}-${editorKey}`}
                                        id={`canvasEditor-${documentKey}-${editorKey}`}
                                        ref={canvasContainerRef}
                                        data-page-scale={editorState?.pageScale ?? ""}
                                        data-page-no={editorState?.pageNo ?? ""}
                                        data-word-count={editorState?.wordCount ?? ""}
                                        style={{
                                            width: "100%",
                                            height: "100%",
                                            display: "flex",
                                            justifyContent: "center",
                                            padding: "0",
                                            backgroundColor: "#dedede",
                                            // F5: 审阅中保留可滚动/可选中，遮罩仅作视觉 tint（见下方 pointerEvents:'none'），
                                            // 不再整体禁用交互，避免 1-2 分钟审阅期间无法阅读合同。
                                            pointerEvents: 'auto',
                                        }}
                                    />
                                    {isLoadingDocument && (
                                        <div
                                            className="absolute inset-0 flex items-center justify-center bg-white bg-opacity-75 z-10">
                                            <div className="text-center">
                                                <p className="text-lg mb-2 text-gray-500">
                                                    正在加载文档...
                                                </p>
                                            </div>
                                        </div>
                                    )}
                                    {documentError && !isLoadingDocument && (
                                        <div
                                            className="absolute inset-0 flex items-center justify-center bg-white bg-opacity-90 z-10">
                                            <div className="text-center max-w-2xl mx-auto p-6">
                                                <p className="text-xl mb-4 text-red-600 font-semibold">
                                                    文档加载失败
                                                </p>
                                                <p className="text-base mb-4 text-gray-700 whitespace-pre-wrap">
                                                    {documentError}
                                                </p>
                                                <Space>
                                                    <Button
                                                        type="primary"
                                                        onClick={() => {
                                                            setDocumentError(null);
                                                            setEditorKey((prev) => prev + 1);
                                                        }}
                                                    >
                                                        重试
                                                    </Button>
                                                    <Button
                                                        type="primary"
                                                        onClick={() => {
                                                            router.push('/')
                                                        }}
                                                    >
                                                        退出
                                                    </Button>
                                                </Space>
                                            </div>
                                        </div>
                                    )}
                                    {isReviewing && !isLoadingDocument && !documentError && (
                                        <div
                                            className="absolute inset-0"
                                            style={{
                                                // F5: 仅视觉提示 tint，不拦截指针事件，保证编辑器可滚动
                                                pointerEvents: 'none',
                                                zIndex: 9999,
                                                backgroundColor: 'rgba(0, 0, 0, 0.2)'
                                            }}
                                        />
                                    )}
                                </>
                            ) : file_url && file_type ? (
                                <div className="flex items-center justify-center h-full text-gray-500">
                                    <div className="text-center">
                                        <p className="text-lg mb-2">正在初始化编辑器...</p>
                                    </div>
                                </div>
                            ) : (
                                <div className="flex items-center justify-center h-full text-gray-500">
                                    <div className="text-center">
                                        <p className="text-lg mb-2">
                                            {!file_url ? "请先上传合同文档" : "文档类型未识别"}
                                        </p>
                                        {!file_url ? (
                                            <Button
                                                size="large"
                                                onClick={() => router.push("/")}
                                                type="primary"
                                            >
                                                返回上一页
                                            </Button>
                                        ) : (
                                            ""
                                        )}
                                        <p className="text-sm text-gray-400">
                                            {!file_url && "文件URL: " + (effectiveData?.file_url || "未设置")}
                                            {file_url &&
                                                !file_type &&
                                                "文件类型: " + (effectiveData?.file_type || "未设置")}
                                        </p>
                                    </div>
                                </div>
                            )}
                        </div>
                    </div>
                )}
            </div>

            {activeTab === 'check' && (
                <div
                    className="bg-gray-100 flex flex-col h-full fixed right-0 top-16 bottom-0 z-[9999] pt-[4rem]"
                    style={{width: "35.13rem", boxSizing: "border-box"}}
                >
                    <div className="flex-1 overflow-auto risk-scroll-area" style={{minHeight: 0}}>
                        {renderSideContent()}
                    </div>
                </div>
            )}

            <LoginModal
                visible={loginVisible}
                onCancel={() => setLoginVisible(false)}
                onSuccess={handleLoginSuccess}
                onSwitchToRegister={() => {
                }}
            />
        </div>
    );
}
