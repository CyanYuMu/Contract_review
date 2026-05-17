'use client'
import React, { useEffect, useMemo, useRef, useState } from 'react';
import type { TableProps } from "antd";
import { Badge, Modal, Table } from "antd";
import dayjs from "dayjs";
import Filter, { FilterType, ReviewFilterValues } from "./Filter";
import { deleteSession } from "@/lib/api/deleteSession";
import type { historyType, RiskResponse } from "@/lib/Interface";
import { getListSession } from "@/lib/api/getListSession";
import { getHistoryDetail } from "@/lib/api/getHistoryDetail";
import { useRouter } from "next/navigation";
import { UploadStore } from "@/store/uploadStore";
import { RiskStore } from "@/store/riskStore";
import { buildStaticFileUrl } from "@/utils/url";
import { getFile } from "@/lib/api/getFile";
import Image from "next/image";
import { assets } from "@/assets/assets";

const initialFilters: ReviewFilterValues = {
    title: "",
    type: undefined,
    status: undefined,
    dateRange: null,
    partyA: "",
    partyB: "",
};

type Props = {
    type?: FilterType;
    onTypeChange?: (type: FilterType) => void;
    onViewRecord?: () => void;
};

export default function ReviewHistory({ type = "Review", onTypeChange, onViewRecord }: Props) {
    const router = useRouter();
    const [filters, setFilters] = useState<ReviewFilterValues>(initialFilters);
    const filtersRef = useRef<ReviewFilterValues>(initialFilters);
    const setRiskDataList = RiskStore((state) => state.setRiskDataList);
    const setStreaming = RiskStore((state) => state.setStreaming);
    const setCompleted = RiskStore((state) => state.setCompleted);
    const normalize = (list: historyType[]) =>
        list.map((item, idx) => ({
            ...item,
            id: item.id ?? idx + 1,
        }));
    const [tableData, setTableData] = useState<historyType[]>(normalize([]));
    const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);
    const [showModal, setShowModal] = useState(false);
    const [pendingDeleteIds, setPendingDeleteIds] = useState<number[]>([]);
    const [rawData, setRawData] = useState<historyType[]>([]);
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(10);
    const [total, setTotal] = useState(0);
    // 用于触发查询的标记
    const [searchTrigger, setSearchTrigger] = useState(0);
    // 防止重复请求
    const isFetching = useRef(false);

    // 本地过滤函数（接收数据参数）
    const applyFiltersToData = (currentFilters: ReviewFilterValues, data: historyType[]) => {
        return data.filter((item) => {
            const matchesName = currentFilters.title
                ? item.title.toLowerCase().includes(currentFilters.title.toLowerCase())
                : true;
            const matchesType = currentFilters.type ? item.session_type === currentFilters.type : true;
            const matchesStatus =
                typeof currentFilters.status === "boolean" ? item.is_accepted === currentFilters.status : true;

            const matchesDate = currentFilters.dateRange && currentFilters.dateRange[0] && currentFilters.dateRange[1]
                ? (() => {
                    const recordDate = dayjs(item.created_at);
                    const [start, end] = currentFilters.dateRange;
                    const afterStart = start ? recordDate.isSame(start) || recordDate.isAfter(start) : true;
                    const beforeEnd = end ? recordDate.isSame(end) || recordDate.isBefore(end) : true;
                    return afterStart && beforeEnd;
                })()
                : true;

            return matchesName && matchesType && matchesStatus && matchesDate;
        });
    };

    useEffect(() => {
        // 防止重复请求
        if (isFetching.current) return;
        isFetching.current = true;

        const parseAccepted = (value: unknown) => {
            if (typeof value === "boolean") return value;
            if (typeof value === "number") return value === 1;
            if (typeof value === "string") {
                const normalized = value.trim().toLowerCase();
                return normalized === "true" || normalized === "1" || normalized === "yes";
            }
            return false;
        };
        const fetchList = async () => {
            try {
                const res = await getListSession({
                    page,
                    page_size: pageSize,
                    session_type: "review",
                });
                const list = res?.data?.sessions ?? [];
                const totalCount = res?.data?.total ?? list.length;
                const normalized = normalize(
                    list.map((item: any, idx: number) => ({
                        id: item.session_id ?? item.id ?? idx + 1,
                        title: item.title || item.file_name || "未命名合同",
                        session_type: item.session_type,
                        created_at: item.created_at,
                        partyA: item.party_a || "未明确",
                        partyB: item.party_b || "未明确",
                        type: item.type || item.contract_type || "未明确",
                        status: parseAccepted(item.is_accepted ?? item.status),
                        is_accepted: parseAccepted(item.is_accepted),
                        file_path: buildStaticFileUrl(item.file_path ?? ""),
                        file_id: item.file_id,
                    }))
                );
                setRawData(normalized);
                // 应用当前筛选条件
                const currentFilters = filtersRef.current;
                const hasFilters = currentFilters.title ||
                    currentFilters.type ||
                    typeof currentFilters.status === "boolean" ||
                    (currentFilters.dateRange && currentFilters.dateRange[0] && currentFilters.dateRange[1]);

                if (hasFilters) {
                    setTableData(applyFiltersToData(currentFilters, normalized));
                } else {
                    setTableData(normalized);
                }
                setTotal(totalCount);
            } finally {
                isFetching.current = false;
            }
        };
        fetchList();
    }, [page, pageSize, searchTrigger]);


    const typeOptions = useMemo(() => {
        const set = new Set(rawData.map((i) => i.session_type).filter(Boolean));
        return Array.from(set).map((value) => ({ value, label: String(value) }));
    }, [rawData]);
    const rowSelection: TableProps<historyType>["rowSelection"] = {
        type: "checkbox",
        selectedRowKeys,
        onChange: (keys, rows) => {
            setSelectedRowKeys(keys);
        },
    };

    const handleFiltersChange = (changed: Partial<ReviewFilterValues>) => {
        const next = { ...filters, ...changed };
        filtersRef.current = next;
        setFilters(next);
    };

    const handleReset = () => {
        setFilters(initialFilters);
        filtersRef.current = initialFilters;
        // 重置后重新请求
        setSearchTrigger(prev => prev + 1);
    };

    const handleSearch = () => {
        // 点击查询时发起请求
        setSearchTrigger(prev => prev + 1);
    };

    const handleDelete = (id: number) => {
        if (!id) return;
        setPendingDeleteIds([id]);
        setShowModal(true);
    };

    const handleBatchDelete = () => {
        const ids = selectedRowKeys.filter((k): k is number => typeof k === "number");
        if (!ids.length) return;
        setPendingDeleteIds(ids);
        setShowModal(true);
    };

    const confirmDelete = async () => {
        if (!pendingDeleteIds.length) {
            setShowModal(false);
            return;
        }
        const ids = pendingDeleteIds.filter((k): k is number => typeof k === "number");
        if (!ids.length) {
            setPendingDeleteIds([]);
            setShowModal(false);
            return;
        }

        try {
            await Promise.all(ids.map((id) => deleteSession(String(id))));
            const idSet = new Set(ids);
            setRawData((prev) => prev.filter((item) => !idSet.has(item.id as number)));
            setTableData((prev) => prev.filter((item) => !idSet.has(item.id as number)));
            setSelectedRowKeys((prev) => prev.filter((k) => !(typeof k === "number" && idSet.has(k))));
        } finally {
            setPendingDeleteIds([]);
            setShowModal(false);
        }
    };

    const handleDownload = async (record: historyType) => {
        if (record.file_id === undefined || record.file_id === null) {
            Modal.error({ title: "导出失败", content: "未找到文件ID" });
            return;
        }
        try {
            const { blob, filename, contentType, size } = await getFile(record.file_id);
            const downloadUrl = URL.createObjectURL(blob);
            const fileNameToUse = filename || (record.title ? record.title : "contract.docx");
            const link = document.createElement("a");
            link.href = downloadUrl;
            link.download = fileNameToUse;
            link.rel = "noopener";
            document.body.appendChild(link);
            link.click();
            link.remove();
            setTimeout(() => URL.revokeObjectURL(downloadUrl), 0);
            void contentType;
            void size;
        } catch (e) {
            const message = e instanceof Error ? e.message : "文件下载失败";
            Modal.error({ title: "导出失败", content: message });
        }
    };

    const cancelDelete = () => {
        setPendingDeleteIds([]);
        setShowModal(false);
    };

    const normalizeRiskList = (raw: unknown, sessionId: number): RiskResponse[] => {
        if (!Array.isArray(raw)) return [];
        return raw.map((item: any, idx: number) => {
            const isAcceptedValue = item.is_accepted;
            const isAccepted =
                typeof isAcceptedValue === "number"
                    ? isAcceptedValue === 1
                    : Boolean(isAcceptedValue);
            return {
                id: Number(item.id ?? item.risk_id ?? idx + 1),
                session_id: Number(item.session_id ?? sessionId),
                task_id: Number(item.task_id ?? 0),
                index: Number(item.index ?? item.risk_index ?? item.order ?? idx + 1),
                original_content: item.original_content ?? item.original ?? "",
                risk_analysis: item.risk_analysis ?? item.analysis ?? "",
                risk_level: item.risk_level ?? item.level ?? "",
                suggested_content: item.suggested_content ?? item.suggestion ?? "",
                is_accepted: isAccepted,
                created_at: item.created_at ?? "",
            };
        });
    };

    const handleView = async (record: historyType) => {
        const sessionId = Number(record.id);
        if (sessionId) {
            try {
                const detail = await getHistoryDetail(String(sessionId));
                const detailData = detail?.data ?? detail;
                const riskListRaw = Array.isArray(detailData)
                    ? detailData
                    : detailData?.risk_points ??
                    detailData?.risk_list ??
                    detailData?.risks ??
                    detailData?.items ??
                    detailData?.list ??
                    detailData?.data ??
                    [];
                const normalizedRisks = normalizeRiskList(riskListRaw, sessionId);
                // 设置风险点数据时，同时记录对应的文档 URL
                setRiskDataList(normalizedRisks, record.file_path);
                setStreaming(false);
                setCompleted(true);
            } catch (e) {
                const message = e instanceof Error ? e.message : "获取审阅结果失败";
                Modal.error({ title: "查看记录失败", content: message });
                return;
            }
        }
        const payload = {
            file_url: record.file_path,
            file_id: record.file_id,
            title: record.title,
            party_a: record.partyA,
            party_b: record.partyB,
        };

        UploadStore.getState().setData(payload);

        localStorage.setItem("uploaded_file_url", payload.file_url ?? "");
        if (payload.file_id !== undefined && payload.file_id !== null) {
            localStorage.setItem("uploaded_file_id", String(payload.file_id));
        }
        localStorage.setItem("uploaded_file_title", payload.title ?? "");
        localStorage.setItem("uploaded_party_a", payload.party_a ?? "");
        localStorage.setItem("uploaded_party_b", payload.party_b ?? "");

        onViewRecord?.();
        router.push("/review");
    };

    const columns = [
        {
            title: "合同名称",
            dataIndex: "title",
            key: "title",
        },
        {
            title: "甲乙方",
            key: "party",
            render: (_: unknown, record: historyType) => (
                <div>
                    <div>{`甲方：${record.partyA || "未明确"}`}</div>
                    <div>{`乙方：${record.partyB || "未明确"}`}</div>
                </div>
            ),
        },
        {
            title: "合同类型",
            dataIndex: "type",
            key: "type",
            render: (value: unknown) => value ?? "未明确",
        },
        {
            title: "修订状态",
            dataIndex: "is_accepted",
            key: "is_accepted",
            render: (_: unknown, record: historyType) => {
                const accepted = record.is_accepted
                return (
                    <Badge
                        status={accepted ? "success" : "error"}
                        text={accepted ? "已修订" : "未修订"}
                    />
                );
            }
        },
        {
            title: "任务创建时间",
            dataIndex: "created_at",
            key: "created_at",
            render: (v: string) => (v ? dayjs(v).format("YYYY/MM/DD HH:mm:ss") : "-"),
        },
        {
            title: "操作",
            dataIndex: "action",
            key: "action",
            render: (_: unknown, record: historyType) => (
                <div className="flex gap-4 text-[#1890ff]">
                    <button
                        type="button"
                        className="bg-transparent border-none p-0 text-[#1890ff] cursor-pointer text-[0.88rem]"
                        onClick={() => { handleView(record) }}
                    >
                        查看记录
                    </button>
                    <button
                        type="button"
                        className="bg-transparent border-none p-0 text-[#1890ff] cursor-pointer text-[0.88rem] font"
                        onClick={() => { void handleDownload(record); }}
                    >
                        导出
                    </button>
                    <button
                        type="button"
                        className="bg-transparent border-none p-0 text-[#1890ff] cursor-pointer text-[0.88rem]"
                        onClick={() => handleDelete(record.id!)}
                    >
                        删除
                    </button>
                </div>
            ),
        }
    ];

    return (
        <div className="flex flex-col">
            <div>
                <Filter
                    filters={filters}
                    onFiltersChange={handleFiltersChange}
                    onReset={handleReset}
                    onSearch={handleSearch}
                    typeOptions={typeOptions}
                    type={type}
                    onTypeChange={onTypeChange}
                />
            </div>
            <div
                className='mt-[1.25rem] mx-[1.25rem] bg-white'
            >
                <div
                    className='text-[1.25rem] text-[black] font-medium pt-[0.75rem] pl-[0.75rem] pb-[0.75rem] bg-white'>审阅记录
                </div>
                {selectedRowKeys.length > 0 && (
                    <div
                        className={`flex items-center justify-between mx-[1.25rem] mb-[1rem] bg-[#e8f3ff] pl-[1.25rem] rounded-[0.19rem] text-[0.88rem] h-[2.81rem] py-[0.75rem]`}
                    >
                        <div className="text-blue-500 flex items-center gap-2">
                            <Image
                                src={assets.Info}
                                alt=""
                                width={20}
                                height={20}
                                className="flex-shrink-0"
                            />
                            <span className='leading-[1.5] mt-[2px]'>
                                已选择 {selectedRowKeys.length} 项
                            </span>
                        </div>
                        <div className="flex items-center gap-4 mr-[0.75rem]">
                            <span
                                className="cursor-pointer text-[#8a8a8a]"
                                onClick={() => {
                                    setSelectedRowKeys([]);
                                }}
                            >
                                取消选择
                            </span>
                            <span
                                className="!text-[#2260F2] cursor-pointer"
                                onClick={handleBatchDelete}
                            >
                                批量删除
                            </span>
                        </div>
                    </div>
                )}
                <div className='px-[1.5rem] bg-white'>
                    <Table
                        columns={columns}
                        dataSource={tableData}
                        rowKey='id'
                        rowSelection={rowSelection}
                        pagination={{
                            current: page,
                            pageSize,
                            total,
                            showSizeChanger: true,
                            onChange: (nextPage, nextPageSize) => {
                                if (nextPageSize !== pageSize) {
                                    // 当每页条数改变时，重置到第一页
                                    setPageSize(nextPageSize);
                                    setPage(1);
                                } else {
                                    // 只改变页码时
                                    setPage(nextPage);
                                }
                            },
                            locale: {
                                items_per_page: '条/页',
                                page: '页',
                            },
                        }}
                    />
                </div>
            </div>
            <Modal
                open={showModal}
                title='删除记录'
                okText='确定'
                cancelText='取消'
                onOk={confirmDelete}
                onCancel={cancelDelete}
            >
                <p>确定删除该记录吗？</p>
            </Modal>
        </div>
    );
}
