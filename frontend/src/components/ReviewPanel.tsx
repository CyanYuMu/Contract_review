"use client";
import React, {useCallback, useEffect, useState} from "react";
import {Button, Form, Input, Radio} from "antd";
import Image from "next/image";
import {useRouter} from "next/navigation";
import toast from "react-hot-toast";
import {startTask} from "@/lib/api/startTask";
import {createSession} from "@/lib/api/createSession";
import {RiskResponse, StartTaskRequest} from "@/lib/Interface";
import {UploadStore} from "@/store/uploadStore";
import {RiskStore} from "@/store/riskStore";
import {assets} from "@/assets/assets";
import { ContractTypeListItem, getContractTypeList } from "@/lib/api/contractType";

type ReviewPanelProps = {
    sessionId?: number;
    onSuccess?: () => void;
    onRiskData?: (risk: RiskResponse) => void;
};

export default function ReviewPanel({
                                        sessionId,
                                        onSuccess,
                                        onRiskData,
                                    }: ReviewPanelProps) {
    const router = useRouter();
    const [loading, setLoading] = useState(false);
    const [form] = Form.useForm();
    const {TextArea} = Input;
    const uploadData = UploadStore((e) => e.data);
    const addRiskData = RiskStore((e) => e.addRiskData);
    const setStreaming = RiskStore((e) => e.setStreaming);
    const setCompleted = RiskStore((e) => e.setCompleted);
    const resetRiskData = RiskStore((e) => e.resetRiskData);
    const setSourceFileUrl = RiskStore((e) => e.setSourceFileUrl);
    const addProgressEvent = RiskStore((e) => e.addProgressEvent);

    const checkHasUploadedContract = useCallback(
        (uploadDataValue?: typeof uploadData) => {
            const data = uploadDataValue ?? uploadData;
            return !!data?.file_url;
        },
        [uploadData]
    );

    const getInitialPartyA = () => {
        const storeData = UploadStore.getState().data;
        if (storeData?.party_a !== undefined && storeData?.party_a !== null) {
            return storeData.party_a;
        }
        if (typeof window !== "undefined") {
            return localStorage.getItem("uploaded_party_a") || "";
        }
        return "";
    };

    const getInitialPartyB = () => {
        const storeData = UploadStore.getState().data;
        if (storeData?.party_b !== undefined && storeData?.party_b !== null) {
            return storeData.party_b;
        }
        if (typeof window !== "undefined") {
            return localStorage.getItem("uploaded_party_b") || "";
        }
        return "";
    };

    const [partyA, setPartyA] = useState<string>(getInitialPartyA);
    const [partyB, setPartyB] = useState<string>(getInitialPartyB);
    const [contractTypes, setContractTypes] = useState<ContractTypeListItem[]>([]);

    const [hasUploadedContract, setHasUploadedContract] = useState(() => {
        const storeData = UploadStore.getState().data;
        return !!storeData?.file_url;
    });

    const resolveDefaultContractType = useCallback((types: ContractTypeListItem[]) => {
        const storedTypeId =
            UploadStore.getState().data?.contract_type_id ??
            (typeof window !== "undefined"
                ? Number(localStorage.getItem("uploaded_contract_type_id") || 0)
                : 0);
        if (storedTypeId) {
            const matched = types.find((item) => Number(item.id) === Number(storedTypeId));
            if (matched) {
                return matched.contractTypeName || matched.name || "";
            }
        }
        return types[0]?.contractTypeName || types[0]?.name || "通用";
    }, []);

    useEffect(() => {
        const fetchContractTypes = async () => {
            try {
                const response = await getContractTypeList();
                if (response?.code === 200 && response?.data?.list) {
                    setContractTypes(response.data.list);
                }
            } catch (error) {
                console.error("获取合同类型失败:", error);
            }
        };
        fetchContractTypes();
    }, []);

    useEffect(() => {
        if (hasUploadedContract) {
            const currentValues = form.getFieldsValue();
            form.setFieldsValue({
                stance: currentValues.stance || "甲方",
                contract_type: currentValues.contract_type || resolveDefaultContractType(contractTypes),
                intensity: currentValues.intensity || "宽松",
            });
        }
    }, [contractTypes, form, hasUploadedContract, resolveDefaultContractType]);

    useEffect(() => {
        const currentUploadData = UploadStore.getState().data;
        const partyAValue =
            currentUploadData?.party_a !== undefined &&
            currentUploadData?.party_a !== null
                ? currentUploadData.party_a
                : typeof window !== "undefined"
                    ? localStorage.getItem("uploaded_party_a") || ""
                    : "";
        const partyBValue =
            currentUploadData?.party_b !== undefined &&
            currentUploadData?.party_b !== null
                ? currentUploadData.party_b
                : typeof window !== "undefined"
                    ? localStorage.getItem("uploaded_party_b") || ""
                    : "";
        setPartyA(partyAValue);
        setPartyB(partyBValue);
        const hasContract = !!currentUploadData?.file_url;
        setHasUploadedContract(hasContract);
    }, [uploadData]);

    const handleSubmit = async () => {
        try {
            setLoading(true);
            toast.loading("提交中…", {id: "create-task-hint"});
            const values = await form.validateFields();
            const storedId =
                typeof window !== "undefined"
                    ? Number(localStorage.getItem("uploaded_file_id") || 0)
                    : 0;

            if (!storedId) {
                toast.error("请先上传合同");
                toast.dismiss("create-task-hint");
                setLoading(false);
                return;
            }

            // 每次审阅创建一个新的 session（独立审阅记录，不覆盖历史审阅）
            let SessionId: number | undefined = sessionId ? Number(sessionId) : undefined;
            if (!SessionId) {
                try {
                    const title =
                        (typeof window !== "undefined"
                            ? localStorage.getItem("uploaded_file_title")
                            : undefined) || "审阅";
                    const sessionRes = await createSession({
                        title,
                        session_type: "review",
                        file_id: storedId,
                    });
                    const sid =
                        sessionRes?.session_id ??
                        sessionRes?.id ??
                        sessionRes?.data?.session_id ??
                        sessionRes?.data?.id;
                    SessionId = Number(sid);
                } catch (e) {
                    console.error("创建审阅会话失败：", e);
                }
            }
            if (!SessionId || !Number.isFinite(SessionId) || SessionId <= 0) {
                toast.error("创建审阅会话失败，请重试");
                toast.dismiss("create-task-hint");
                setLoading(false);
                return;
            }
            if (typeof window !== "undefined") {
                localStorage.setItem("review_session_id", String(SessionId));
            }

            // 获取文件 URL
            const fileUrl =
                uploadData?.file_url ||
                (typeof window !== "undefined"
                    ? localStorage.getItem("uploaded_file_url") || undefined
                    : undefined);
            const fileType = uploadData?.file_type;

            if (!fileUrl) {
                toast.error("文件地址不存在，请重新上传");
                toast.dismiss("create-task-hint");
                setLoading(false);
                return;
            }

            const payload: StartTaskRequest = {
                session_id: SessionId,
                stance: values.stance,
                contract_type: values.contract_type,
                intensity: values.intensity ?? "标准",
                description: values.description || null,
            };

            const startKey = "start-task";
            toast.loading("正在启动审查任务...", {id: startKey});

            resetRiskData();
            // 记录当前审查对应的文档 URL
            setSourceFileUrl(fileUrl ?? null);
            setCompleted(false);
            setStreaming(true);
            addProgressEvent({
                phase: "prepare",
                agent: "ReviewPanel",
                status: "running",
                message: "正在启动合同审阅...",
                progress: 0.01,
                timestamp: new Date().toISOString(),
                data: {event_type: "client_start"},
            });
            if (typeof window !== "undefined") {
                localStorage.setItem("review_workspace_active", "1");
                localStorage.setItem("uploaded_file_url", fileUrl);
                if (fileType) {
                    localStorage.setItem("uploaded_file_type", fileType);
                }
            }

            // 立即跳转到审阅页，避免停留在首页阻塞等待 SSE 连接（SSE 建立前先展示"审阅中"）。
            // 连接失败由 startTask 的 onError/.catch 在审阅页内处理。
            router.push("/review");

            toast.success("任务已启动", {id: startKey});
            toast.dismiss("create-task-hint");
            setLoading(false);
            if (onSuccess) onSuccess();

            startTask(
                {
                    session_id: payload.session_id,
                    stance: payload.stance,
                    contract_type: payload.contract_type,
                    intensity: payload.intensity,
                    description: payload.description ?? null,
                },
                () => {
                    // SSE 已连接，导航已在上方完成，此处无需再次跳转
                },
                (risk) => {
                    addRiskData(risk);
                    if (onRiskData) {
                        onRiskData(risk);
                    }
                },
                undefined,
                undefined,
                () => {
                    setStreaming(true);
                },
                () => {
                    setStreaming(false);
                },
                () => {
                    setCompleted(true);
                },
                (event) => {
                    addProgressEvent(event);
                }
            ).catch((err) => {
                addProgressEvent({
                    phase: "prepare",
                    agent: "ReviewPanel",
                    status: "failed",
                    message: "审阅任务启动失败",
                    progress: 0,
                    timestamp: new Date().toISOString(),
                    data: {event_type: "client_error"},
                });
                setStreaming(false);
                setCompleted(false);
                const fallback = "操作失败";
                if (err instanceof Error) {
                    toast.error(err.message || fallback);
                } else {
                    toast.error(fallback);
                }
            });
        } catch (err) {
            const fallback = "操作失败";
            if (err instanceof Error) {
                toast.error(err.message || fallback);
            } else {
                toast.error(fallback);
            }
            toast.dismiss("create-task-hint");
            setLoading(false);
        }
    };

    if (!hasUploadedContract) {
        return (
            <div
                className="bg-white border-[0.06rem] border-[#e3e3e3] h-screen flex items-center justify-center mr-[2.25rem] rounded-[0.31rem] w-[78.88rem]"
                style={{width: "34.13rem"}}
            >
                <div className="text-center p-4 flex flex-col items-center">
                    <Image
                        src={assets.IntroduceIcon}
                        alt="请先上传合同"
                        width={300}
                        height={300}
                        style={{
                            maxWidth: "100%",
                            height: "auto",
                            display: "block",
                            transform: "translateY(-3rem)",
                        }}
                        priority
                    />
                </div>
            </div>
        );
    }

    return (
        <div
            className="bg-white  h-screen overflow-y-auto border-[0.06rem] border-[#e3e3e3] rounded-[0.31rem] mr-[2.25rem]"
            style={{width: "34.13rem"}}
        >
            <div className="pt-[1.56rem]">
                <div
                    className="text-[1.25rem] text-[black] font-bold border-l-4 border-[#155dfc] pl-[0.63rem] mb-[3.13rem] ml-[2.5rem]">
                    <h1>审查设置</h1>
                </div>
                <Form
                    className="flex flex-col gap-4"
                    size="large"
                    form={form}
                    colon={false}
                    initialValues={{stance: "甲方", contract_type: "", intensity: "宽松"}}
                >
                    <div>
                        <div className="mb-2 text-[1.13rem] ml-[5.5rem]">审查立场</div>
                        <Form.Item name="stance" style={{marginBottom: 0}}>
                            <Radio.Group
                                style={{
                                    fontSize: 16,
                                    display: "flex",
                                    flexDirection: "column",
                                    rowGap: 10,
                                }}
                            >
                                <Radio value="甲方" className="!ml-[11.06rem]">
                  <span>
                    甲方：
                    <span className="text-gray-600">
                      {partyA || "{未明确}"}
                    </span>
                  </span>
                                </Radio>
                                <Radio value="乙方" className="!ml-[11.06rem]">
                  <span>
                    乙方：
                    <span className="text-gray-600">
                      {partyB || "{未明确}"}
                    </span>
                  </span>
                                </Radio>
                            </Radio.Group>
                        </Form.Item>
                    </div>
                    <div>
                        <div className="mb-2 text-[1.13rem] ml-[5.5rem]">合同类型</div>
                        <Form.Item name="contract_type" style={{marginBottom: 0}}>
                            <Radio.Group
                                style={{
                                    fontSize: 16,
                                    display: "flex",
                                    flexDirection: "column",
                                    rowGap: 10,
                                }}
                            >
                                {(contractTypes.length > 0 ? contractTypes : [{ id: "fallback", contractTypeName: "通用", creator: "", updateDate: "" }]).map((item) => {
                                    const name = item.contractTypeName || item.name || "通用";
                                    return (
                                        <Radio key={item.id} value={name} className="!ml-[11.06rem]">
                                            {name}
                                        </Radio>
                                    );
                                })}
                            </Radio.Group>
                        </Form.Item>
                    </div>
                    <Form.Item style={{marginBottom: 0, marginTop: 8}}>
                        <div className="flex gap-[3.56rem] justify-center">
                            <Button
                                className="!w-[9.75rem] !h-[3.13rem]"
                                style={{backgroundColor: "white"}}
                                onClick={() => form.resetFields()}
                            >
                                重置选项
                            </Button>
                            <Button
                                className="!w-[9.75rem] !h-[3.13rem]"
                                loading={loading}
                                onClick={handleSubmit}
                                style={{backgroundColor: "#2260f2", color: "white"}}
                            >
                                开始审查
                            </Button>
                        </div>
                    </Form.Item>
                </Form>
            </div>
        </div>
    );
}
