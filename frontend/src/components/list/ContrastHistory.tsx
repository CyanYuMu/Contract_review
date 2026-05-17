'use client';
import React, { useEffect, useMemo, useRef, useState } from "react";
import type { TableProps } from "antd";
import { Badge, Modal, Table } from "antd";
import dayjs from "dayjs";
import Filter, { FilterType, ReviewFilterValues, } from "./Filter";
import { getListSession } from "@/lib/api/getListSession";
import type { contrastType } from "@/lib/Interface";
import { deleteSession } from "@/lib/api/deleteSession";
import toast from "react-hot-toast";
import Image from "next/image";
import { assets } from "@/assets/assets";
import { useRouter } from "next/navigation";
import { ContrastuploadStore } from "@/store/ContrastuploadStore";
import { buildStaticFileUrl, resolveFileUrl } from "@/utils/url";
import { getHistoryDetail } from "@/lib/api/getHistoryDetail";


const initialFilters: ReviewFilterValues = {
    title: "",
    type: undefined,
    status: undefined,
    dateRange: null,
};

type Props = {
    type?: FilterType;
    onTypeChange?: (type: FilterType) => void;
};

type CompareFileInfo = {
    file_id?: number;
    title?: string;
    file_path?: string;
    download_url?: string;
};

type CompareSessionListItem = {
    session_id?: number;
    id?: number;
    file_name?: string;
    file_name_1?: string;
    file_name_2?: string;
    similarity?: number;
    status?: string;
    created_at?: string;
    file_path?: string;
    file_path_1?: string;
    file_path_2?: string;
    original_file_path?: string;
    comparison_file_path?: string;
    file_id?: number;
    file_id_1?: number;
    file_id_2?: number;
    original_file_id?: number;
    comparison_file_id?: number;
    download_url?: string;
    download_url_2?: string;
    standard_download_url?: string;
    comparison_download_url?: string;
    standard_file?: CompareFileInfo;
    comparison_file?: CompareFileInfo;
};

type CompareHistoryPayload = {
    session_id?: number;
    diffs?: unknown[];
    standard_file?: CompareFileInfo;
    comparison_file?: CompareFileInfo;
};

export default function ContrastHistory({
    type = "Contrast",
    onTypeChange,
}: Props) {
    const router = useRouter();
    const [filters, setFilters] =
        useState<ReviewFilterValues>(initialFilters);
    const filtersRef = useRef<ReviewFilterValues>(initialFilters);

    // current 分支里的 normalize 保留
    const normalize = (list: contrastType[]) =>
        list.map((item, idx) => ({
            ...item,
            id: item.id ?? idx + 1,
        }));

    // 用 normalize 后的数据初始化
    const [tableData, setTableData] = useState<contrastType[]>(
        normalize([])
    );
    const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);
    const [showModal, setShowModal] = useState(false);
    const [pendingDeleteIds, setPendingDeleteIds] = useState<React.Key[]>([]);
    const [rawData, setRawData] = useState<contrastType[]>([]);
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(10);
    const [total, setTotal] = useState(0);
    // 用于触发查询的标记
    const [searchTrigger, setSearchTrigger] = useState(0);
    // 防止重复请求
    const isFetching = useRef(false);
    const safeBuildStaticFileUrl = (filePath?: string) => {
        return filePath ? buildStaticFileUrl(filePath) : "";
    };

    // 本地过滤函数（接收数据参数）
    const applyFiltersToData = (currentFilters: ReviewFilterValues, data: contrastType[]) => {
        return data.filter((item) => {
            const matchesName = currentFilters.title
                ? item.origin_contract_name
                    .toLowerCase()
                    .includes(currentFilters.title.toLowerCase()) ||
                item.new_contract_name
                    .toLowerCase()
                    .includes(currentFilters.title.toLowerCase())
                : true;

            const matchesStatus =
                typeof currentFilters.status === "boolean"
                    ? item.status === currentFilters.status
                    : true;

            const matchesDate =
                currentFilters.dateRange &&
                    currentFilters.dateRange[0] &&
                    currentFilters.dateRange[1]
                    ? (() => {
                        const recordDate = dayjs(item.dateRange);
                        const [start, end] = currentFilters.dateRange;
                        const afterStart = start
                            ? recordDate.isSame(start) ||
                            recordDate.isAfter(start)
                            : true;
                        const beforeEnd = end
                            ? recordDate.isSame(end) ||
                            recordDate.isBefore(end)
                            : true;
                        return afterStart && beforeEnd;
                    })()
                    : true;

            return matchesName && matchesStatus && matchesDate;
        });
    };

    useEffect(() => {
        // 防止重复请求
        if (isFetching.current) return;
        isFetching.current = true;

        const fetchList = async () => {
            try {
                const res = await getListSession({
                    page,
                    page_size: pageSize,
                    session_type: "compare",
                });

                const responseData = res?.data ?? res;
                const listData =
                    responseData?.sessions ??
                    responseData?.data?.sessions ??
                    responseData?.data?.data?.sessions ??
                    [];
                const list: CompareSessionListItem[] = Array.isArray(listData) ? listData : [];
                const totalCount =
                    responseData?.total ??
                    responseData?.data?.total ??
                    responseData?.data?.data?.total ??
                    list.length;
                const normalized = normalize(
                    list.map((item, idx: number) => {
                        const standardFile = item.standard_file ?? {};
                        const comparisonFile = item.comparison_file ?? {};
                        return {
                            id: item.session_id ?? item.id ?? idx + 1,
                            origin_contract_name: item.file_name_1 ?? item.file_name ?? standardFile.title ?? "",
                            new_contract_name: item.file_name_2 ?? comparisonFile.title ?? "",
                            similarity: item.similarity ?? 0,
                            status: item.status ? item.status === "completed" : Boolean(item.file_id_1 && item.file_id_2),
                            dateRange: item.created_at ?? "",
                            file_path: item.file_path_1 ?? item.file_path ?? standardFile.file_path ?? item.original_file_path ?? "",
                            file_path_2: item.file_path_2 ?? comparisonFile.file_path ?? item.comparison_file_path ?? "",
                            file_id: item.file_id_1 ?? item.file_id ?? standardFile.file_id ?? item.original_file_id,
                            file_id_2: item.file_id_2 ?? comparisonFile.file_id ?? item.comparison_file_id,
                            standard_download_url: item.download_url ?? standardFile.download_url ?? item.standard_download_url,
                            comparison_download_url: item.download_url_2 ?? comparisonFile.download_url ?? item.comparison_download_url,
                        };
                    })
                );

                setRawData(normalized);
                // 应用当前筛选条件
                const currentFilters = filtersRef.current;
                const hasFilters = currentFilters.title ||
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
    const typeOptions = useMemo(() => [], []);

    const rowSelection: TableProps<contrastType>["rowSelection"] = {
        type: "checkbox",
        selectedRowKeys,
        onChange: (keys) => {
            setSelectedRowKeys(keys);
        },
    };

    const handleFiltersChange = (
        changed: Partial<ReviewFilterValues>
    ) => {
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

    const handleDelete = (id: React.Key) => {
        setPendingDeleteIds([id]);
        setShowModal(true);
    };

    const handleBatchDelete = () => {
        if (!selectedRowKeys.length) return;
        setPendingDeleteIds(selectedRowKeys);
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
            toast.success("删除成功！")
        } finally {
            setPendingDeleteIds([]);
            setShowModal(false);
        }
    };


    const cancelDelete = () => {
        setPendingDeleteIds([]);
        setShowModal(false);
    };

    const handleView = async (record: contrastType) => {
        const sessionId = Number(record.id);
        let detailPayload: CompareHistoryPayload | null = null;

        if (sessionId) {
            try {
                const detail = await getHistoryDetail(String(sessionId));
                const detailData = detail?.data ?? detail;
                detailPayload = detailData?.data ?? detailData;
                if (detailPayload) {
                    localStorage.setItem("comparison_history_detail", JSON.stringify(detailPayload));
                    localStorage.setItem("comparison_result", JSON.stringify(detailPayload));
                }
            } catch (e) {
                const message = e instanceof Error ? e.message : "获取比对记录失败";
                Modal.error({ title: "查看记录失败", content: message });
                return;
            }
        }

        const standardFile = detailPayload?.standard_file ?? {};
        const comparisonFile = detailPayload?.comparison_file ?? {};
        const originalFileUrl =
            safeBuildStaticFileUrl(standardFile.file_path ?? record.original_file_path ?? record.file_path ?? "") ||
            resolveFileUrl(standardFile.download_url ?? record.standard_download_url ?? "");
        const comparisonFileUrl =
            safeBuildStaticFileUrl(comparisonFile.file_path ?? record.comparison_file_path ?? record.file_path_2 ?? "") ||
            resolveFileUrl(comparisonFile.download_url ?? record.comparison_download_url ?? "");

        if (!originalFileUrl || !comparisonFileUrl) {
            Modal.error({ title: "查看记录失败", content: "未找到比对文件路径" });
            return;
        }

        ContrastuploadStore.getState().setOriginalFile({
            file_url: originalFileUrl,
            title: record.origin_contract_name || standardFile.title,
            file_id: record.original_file_id ?? record.file_id ?? standardFile.file_id,
        });
        ContrastuploadStore.getState().setComparisonFile({
            file_url: comparisonFileUrl,
            title: record.new_contract_name || comparisonFile.title,
            file_id: record.comparison_file_id ?? record.file_id_2 ?? comparisonFile.file_id,
        });

        localStorage.setItem("original_file_url", originalFileUrl);
        localStorage.setItem("comparison_file_url", comparisonFileUrl);
        localStorage.setItem("original_file_title", record.origin_contract_name ?? "");
        localStorage.setItem("comparison_file_title", record.new_contract_name ?? "");
        localStorage.setItem("contrast_workspace_active", "1");
        if (sessionId) {
            localStorage.setItem("comparison_session_id", String(sessionId));
        }
        if (record.original_file_id ?? record.file_id) {
            localStorage.setItem("original_file_id", String(record.original_file_id ?? record.file_id));
        }
        if (record.comparison_file_id ?? record.file_id_2) {
            localStorage.setItem("comparison_file_id", String(record.comparison_file_id ?? record.file_id_2));
        }
        if ((record.original_file_id ?? record.file_id) && (record.comparison_file_id ?? record.file_id_2)) {
            localStorage.setItem(
                "comparison_pair_key",
                `${record.original_file_id ?? record.file_id}:${record.comparison_file_id ?? record.file_id_2}`
            );
        }

        router.push("/result");
    };

    const handleDownload = (record: contrastType) => {
        const comparisonFileUrl = resolveFileUrl(
            record.comparison_download_url ?? record.download_url_2 ?? ""
        );
        const fallbackFileUrl = buildStaticFileUrl(
            record.comparison_file_path ?? record.file_path_2 ?? ""
        );

        const downloadUrl = comparisonFileUrl || fallbackFileUrl;

        if (!downloadUrl) {
            Modal.error({ title: "导出失败", content: "未找到比对合同路径" });
            return;
        }

        const link = document.createElement("a");
        link.href = downloadUrl;
        link.download = record.new_contract_name || "contrast.docx";
        link.rel = "noopener";
        document.body.appendChild(link);
        link.click();
        link.remove();
    };

    const columns = [
        {
            title: "标准合同",
            dataIndex: "origin_contract_name",
            key: "origin_contract",
        },
        {
            title: "比对合同",
            dataIndex: "new_contract_name",
            key: "new_contract",
        },
        {
            title: "相似度",
            dataIndex: "similarity",
            key: "similarity",
            render: (value: number) => `${value}%`,
        },
        {
            title: "审核状态",
            dataIndex: "status",
            key: "status",
            render: (value: boolean) => (
                <Badge
                    status={value ? "success" : "error"}
                    text={value ? "已审核" : "未审核"}
                />
            ),
        },
        {
            title: "任务创建时间",
            dataIndex: "dateRange",
            key: "dateRange",
            render: (v: string) => (v ? dayjs(v).format("YYYY/MM/DD HH:mm:ss") : "-"),
        },
        {
            title: "操作",
            dataIndex: "action",
            key: "action",
            render: (_: unknown, record: contrastType) => (
                <div className="flex gap-4 text-[#1890ff]">
                    <button
                        type="button"
                        className="bg-transparent border-none p-0 text-[#1890ff] cursor-pointer text-[0.88rem]"
                        onClick={() => handleView(record)}
                    >
                        查看记录
                    </button>
                    <button
                        type="button"
                        className="bg-transparent border-none p-0 text-[#1890ff] cursor-pointer text-[0.88rem] font"
                        onClick={() => handleDownload(record)}
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
        },
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

            <div className="mt-[1.25rem] mx-[1.25rem] bg-white">
                <div className='text-[1.25rem] text-[black] font-medium pt-[0.75rem] pl-[0.75rem] pb-[0.75rem] bg-white'>
                    比对记录
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

                <div className="px-[1.5rem] bg-white">
                    <Table
                        columns={columns}
                        dataSource={tableData}
                        rowKey={(record, index) =>
                            record.id ?? `${record.origin_contract_name}-${record.new_contract_name}-${index}`
                        }
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
                                items_per_page: "条/页",
                                page: "页",
                            },
                        }}
                    />
                </div>
            </div>

            <Modal
                open={showModal}
                title="删除记录"
                okText="确定"
                cancelText="取消"
                onOk={confirmDelete}
                onCancel={cancelDelete}
            >
                <p>确定删除该记录吗？</p>
            </Modal>
        </div>
    );
}
