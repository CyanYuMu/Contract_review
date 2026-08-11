'use client';

import React, { Suspense, useCallback, useEffect, useMemo, useState } from 'react';
import { Button, DatePicker, Input, Modal, Select, Space, Table, Tag, message } from 'antd';
import type { TableColumnsType } from 'antd';
import dayjs from 'dayjs';
import Image from "next/image";
import { useRouter, useSearchParams } from 'next/navigation';
import { assets } from "@/assets/assets";
import { useRiskEditStore } from '@/store/riskEditStore';
import { CustomPagination } from '@/components/table/CustomPagination';
import { ContractTypeListItem, getContractTypeList } from '@/lib/api/contractType';
import {
    RiskPointListItem,
    RiskPointStats,
    batchDeleteRiskPoint,
    deleteRiskPoint,
    getRiskPointPage,
    getRiskPointStats,
} from '@/lib/api/risk';

const { RangePicker } = DatePicker;

const emptyStats: RiskPointStats = {
    total: 0,
    enabled: 0,
    disabled: 0,
    indexed: 0,
    byLevel: [],
    byType: [],
    byContractType: [],
};

function RiskPageContent() {
    const [riskId, setRiskId] = useState<string>('');
    const [riskContent, setRiskContent] = useState<string>('');
    const [status, setStatus] = useState<string>('');
    const [contractType, setContractType] = useState<string>('');
    const [creator, setCreator] = useState<string>('');
    const [dateRange, setDateRange] = useState<[dayjs.Dayjs, dayjs.Dayjs] | null>(null);
    const [dataSource, setDataSource] = useState<RiskPointListItem[]>([]);
    const [contractTypes, setContractTypes] = useState<ContractTypeListItem[]>([]);
    const [stats, setStats] = useState<RiskPointStats>(emptyStats);
    const [loading, setLoading] = useState<boolean>(false);
    const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);
    const [pagination, setPagination] = useState({
        current: 1,
        pageSize: 10,
        total: 0
    });
    const router = useRouter();
    const searchParams = useSearchParams();
    const setRiskData = useRiskEditStore((state) => state.setRiskData);

    useEffect(() => {
        const contractTypeParam = searchParams.get('contractType');
        if (contractTypeParam) {
            setContractType(contractTypeParam);
        }
    }, [searchParams]);

    const fetchContractTypes = useCallback(async () => {
        try {
            const response = await getContractTypeList();
            if (response?.code === 200 && response?.data?.list) {
                setContractTypes(response.data.list);
            }
        } catch (error) {
            console.error('获取合同类型失败:', error);
        }
    }, []);

    const fetchStats = useCallback(async (type = contractType) => {
        try {
            const response = await getRiskPointStats(type || undefined);
            if (response?.code === 200 && response?.data) {
                setStats(response.data);
            } else {
                setStats(emptyStats);
            }
        } catch {
            setStats(emptyStats);
        }
    }, [contractType]);

    const fetchRiskList = useCallback(async (
        page: number,
        pageSize: number,
        type = contractType,
    ) => {
        setLoading(true);
        try {
            const response = await getRiskPointPage({
                riskId: riskId || undefined,
                riskContent: riskContent || undefined,
                status: status || undefined,
                contractType: type || undefined,
                creator: creator || undefined,
                startDate: dateRange?.[0]?.format('YYYY-MM-DD'),
                endDate: dateRange?.[1]?.format('YYYY-MM-DD'),
                page,
                pageSize,
            });

            if (response?.code === 200 && response?.data) {
                setDataSource(response.data.list || []);
                setPagination({
                    current: response.data.page || page,
                    pageSize: response.data.pageSize || response.data.page_size || pageSize,
                    total: response.data.total || 0,
                });
            } else {
                setDataSource([]);
                setPagination((prev) => ({ ...prev, total: 0 }));
            }
        } catch {
            message.error('获取数据失败');
            setDataSource([]);
        } finally {
            setLoading(false);
        }
    }, [contractType, creator, dateRange, riskContent, riskId, status]);

    useEffect(() => {
        fetchContractTypes();
    }, [fetchContractTypes]);

    useEffect(() => {
        fetchRiskList(1, pagination.pageSize, contractType);
        fetchStats(contractType);
    }, [contractType, fetchRiskList, fetchStats, pagination.pageSize]);

    useEffect(() => {
        const refresh = searchParams.get('refresh');
        if (refresh) {
            fetchRiskList(1, pagination.pageSize, contractType);
            fetchStats(contractType);
        }
    }, [searchParams, fetchRiskList, fetchStats, pagination.pageSize, contractType]);

    const handleAdd = () => {
        const query = contractType ? `?mode=add&contractType=${encodeURIComponent(contractType)}` : '?mode=add';
        router.push(`/setting/risk/editAndAdd${query}`);
    };

    const handleSearch = () => {
        fetchRiskList(1, pagination.pageSize, contractType);
        fetchStats(contractType);
    };

    const handleReset = () => {
        setRiskId('');
        setRiskContent('');
        setStatus('');
        setContractType('');
        setCreator('');
        setDateRange(null);
        setTimeout(() => {
            fetchRiskList(1, pagination.pageSize, '');
            fetchStats('');
        }, 0);
    };

    const handleEdit = (record: RiskPointListItem) => {
        setRiskData({
            id: String(record.id),
            riskId: record.riskId,
            contractTypeId: record.contractTypeId,
            contractType: record.contractType,
            applicableScope: record.applicableScope,
            department: record.department,
            riskContent: record.riskContent,
            riskType: record.riskType,
            riskLevel: record.riskLevel,
            isEnabled: record.isEnabled
        });
        router.push(`/setting/risk/editAndAdd?mode=edit&id=${record.id}`);
    };

    const handleDelete = (record: RiskPointListItem) => {
        Modal.confirm({
            title: '删除风险点',
            content: '确认要删除此条数据吗？删除后对应 RAG 知识也会同步移除。',
            okText: '确认',
            cancelText: '取消',
            okButtonProps: { danger: true },
            onOk: async () => {
                try {
                    const response = await deleteRiskPoint(record.id);
                    if (response?.code === 200) {
                        message.success(`删除成功: ${record.riskId}`);
                        await fetchRiskList(pagination.current, pagination.pageSize, contractType);
                        await fetchStats(contractType);
                    } else {
                        message.error(response?.msg || '删除失败');
                    }
                } catch {
                    message.error('删除失败');
                }
            },
        });
    };

    const handleBatchDelete = () => {
        if (selectedRowKeys.length === 0) {
            message.warning('请选择要删除的项');
            return;
        }
        Modal.confirm({
            title: '批量删除风险点',
            content: `确认要删除选中的 ${selectedRowKeys.length} 项吗？`,
            okText: '确认',
            cancelText: '取消',
            okButtonProps: { danger: true },
            onOk: async () => {
                try {
                    const ids = selectedRowKeys.map((key) => Number(key));
                    const response = await batchDeleteRiskPoint(ids);
                    if (response?.code === 200) {
                        message.success(`批量删除成功: ${selectedRowKeys.length} 项`);
                        setSelectedRowKeys([]);
                        await fetchRiskList(pagination.current, pagination.pageSize, contractType);
                        await fetchStats(contractType);
                    } else {
                        message.error(response?.msg || '删除失败');
                    }
                } catch {
                    message.error('删除失败');
                }
            },
        });
    };

    const handlePageChange = (page: number, pageSize: number) => {
        fetchRiskList(page, pageSize, contractType);
    };

    const levelStats = useMemo(() => {
        const map = new Map(stats.byLevel.map((item) => [item.name, item.value]));
        return [
            { label: '高', color: '#ff4d4f', value: map.get('高') || 0 },
            { label: '中', color: '#faad14', value: map.get('中') || 0 },
            { label: '低', color: '#52c41a', value: map.get('低') || 0 },
        ];
    }, [stats.byLevel]);

    const columns: TableColumnsType<RiskPointListItem> = [
        {
            title: '风险点ID',
            dataIndex: 'riskId',
            key: 'riskId',
            width: 130
        },
        {
            title: '风险点内容',
            dataIndex: 'riskContent',
            key: 'riskContent',
            ellipsis: true
        },
        {
            title: '所属合同类型',
            dataIndex: 'applicableContractType',
            key: 'applicableContractType',
            width: 150
        },
        {
            title: '风险等级',
            dataIndex: 'riskLevel',
            key: 'riskLevel',
            width: 100,
            render: (value: string) => (
                <Tag color={value === '高' ? 'red' : value === '中' ? 'gold' : 'green'}>{value}</Tag>
            )
        },
        {
            title: 'RAG状态',
            dataIndex: 'ragStatus',
            key: 'ragStatus',
            width: 100,
            render: (value: string) => (
                <Tag color={value === '已入库' ? 'blue' : value === '已停用' ? 'default' : 'orange'}>{value}</Tag>
            )
        },
        {
            title: '创建者',
            dataIndex: 'creator',
            key: 'creator',
            width: 100
        },
        {
            title: '更改日期',
            dataIndex: 'updateDate',
            key: 'updateDate',
            width: 180
        },
        {
            title: '状态',
            dataIndex: 'status',
            key: 'status',
            width: 90
        },
        {
            title: '操作',
            key: 'action',
            width: 150,
            render: (_, record) => (
                <Space size="small">
                    <Button type="link" size="small" onClick={() => handleEdit(record)}>
                        编辑
                    </Button>
                    <Button type="link" danger size="small" onClick={() => handleDelete(record)}>
                        删除
                    </Button>
                </Space>
            )
        }
    ];

    const rowSelection = {
        selectedRowKeys,
        onChange: (newSelectedRowKeys: React.Key[]) => {
            setSelectedRowKeys(newSelectedRowKeys);
        }
    };

    return (
        <div className="flex flex-col bg-[#f1f1f1] h-[100%]">
            <div className="mb-4 bg-white p-[1rem] rounded">
                <div className="grid grid-cols-4 gap-x-4 gap-y-4 mb-4">
                    <div className="flex items-center">
                        <span className="text-gray-700 w-[6rem] text-left mr-2">风险点ID：</span>
                        <Input
                            placeholder="请输入风险点ID"
                            value={riskId}
                            onChange={(e) => setRiskId(e.target.value)}
                            className="flex-1"
                        />
                    </div>
                    <div className="flex items-center col-span-2">
                        <span className="text-gray-700 w-[6rem] text-left mr-2">风险点内容：</span>
                        <Input
                            placeholder="请输入风险点内容"
                            value={riskContent}
                            onChange={(e) => setRiskContent(e.target.value)}
                            className="flex-1"
                        />
                    </div>
                    <div className="flex items-center">
                        <span className="text-gray-700 w-[6rem] text-left mr-2">状态：</span>
                        <Select
                            placeholder="请选择"
                            value={status || undefined}
                            onChange={setStatus}
                            className="flex-1"
                            allowClear
                        >
                            <Select.Option value="enabled">启用</Select.Option>
                            <Select.Option value="disabled">停用</Select.Option>
                        </Select>
                    </div>
                </div>
                <div className="grid grid-cols-4 gap-x-4 gap-y-4">
                    <div className="flex items-center">
                        <span className="text-gray-700 w-[6rem] text-left mr-2">合同类型：</span>
                        <Select
                            placeholder="请选择合同类型"
                            value={contractType || undefined}
                            onChange={setContractType}
                            className="flex-1"
                            allowClear
                        >
                            {contractTypes.map((item) => (
                                <Select.Option key={item.id} value={item.contractTypeName || item.name}>
                                    {item.contractTypeName || item.name}
                                </Select.Option>
                            ))}
                        </Select>
                    </div>
                    <div className="flex items-center">
                        <span className="text-gray-700 w-[6rem] text-left mr-2">创建者：</span>
                        <Input
                            placeholder="请输入创建者"
                            value={creator}
                            onChange={(e) => setCreator(e.target.value)}
                            className="flex-1"
                        />
                    </div>
                    <div className="flex items-center">
                        <span className="text-gray-700 w-[6rem] text-left mr-2">更改日期：</span>
                        <RangePicker
                            value={dateRange}
                            onChange={(dates) => setDateRange(dates as [dayjs.Dayjs, dayjs.Dayjs] | null)}
                            format="YYYY/MM/DD"
                            className="flex-1"
                            placeholder={['开始日期', '结束日期']}
                        />
                    </div>
                    <div className="flex justify-end gap-2">
                        <Button onClick={handleReset}>重置</Button>
                        <Button type="primary" onClick={handleSearch}>
                            查询
                        </Button>
                    </div>
                </div>
            </div>

            <div className="grid grid-cols-4 gap-4 mb-4">
                <div className="bg-white rounded p-4">
                    <div className="text-sm text-gray-500">风险点总数</div>
                    <div className="text-2xl font-semibold text-[#1f1f1f]">{stats.total}</div>
                </div>
                <div className="bg-white rounded p-4">
                    <div className="text-sm text-gray-500">已启用</div>
                    <div className="text-2xl font-semibold text-[#2260F2]">{stats.enabled}</div>
                </div>
                <div className="bg-white rounded p-4">
                    <div className="text-sm text-gray-500">已入库</div>
                    <div className="text-2xl font-semibold text-[#13a872]">{stats.indexed}</div>
                </div>
                <div className="bg-white rounded p-4">
                    <div className="text-sm text-gray-500 mb-2">等级分布</div>
                    <div className="flex h-3 overflow-hidden rounded bg-[#f0f0f0]">
                        {levelStats.map((item) => {
                            const width = stats.total > 0 ? `${(item.value / stats.total) * 100}%` : '0%';
                            return (
                                <div
                                    key={item.label}
                                    title={`${item.label}风险 ${item.value}`}
                                    style={{ width, backgroundColor: item.color }}
                                />
                            );
                        })}
                    </div>
                    <div className="mt-2 flex gap-3 text-xs text-gray-500">
                        {levelStats.map((item) => (
                            <span key={item.label}>{item.label}:{item.value}</span>
                        ))}
                    </div>
                </div>
            </div>

            <div className="bg-white flex-1 flex flex-col overflow-hidden p-[1rem] relative pb-0 rounded">
                <div className="mb-4 flex items-center justify-between">
                    <div className="text-[1.25rem] text-black font-bold">
                        风险点列表
                    </div>
                    <Button type="primary" onClick={handleAdd}>
                        新增风险点
                    </Button>
                </div>

                {selectedRowKeys.length > 0 && (
                    <div className="mb-4 flex items-center">
                        <div className="text-sm bg-[#ebf3ff] w-[100%] py-[0.5rem] px-[1rem] flex">
                            <div className="text-blue-500 flex items-center">
                                <Image
                                    src={assets.Info}
                                    alt=""
                                    width={20}
                                    height={20}
                                />
                                <span className='ml-[0.5rem]'>
                                    已选择 {selectedRowKeys.length} 项
                                </span>
                            </div>
                            <span className='ml-[auto]'>
                                <Button type="link" style={{ color: 'gray' }} size="small" onClick={() => setSelectedRowKeys([])}>
                                    取消选择
                                </Button>
                                <Button type="link" size="small" onClick={handleBatchDelete}>
                                    批量删除
                                </Button>
                            </span>
                        </div>
                    </div>
                )}

                <div className="flex-1 overflow-y-auto">
                    <Table
                        rowSelection={rowSelection}
                        rowKey="id"
                        columns={columns}
                        dataSource={dataSource}
                        loading={loading}
                        pagination={false}
                    />
                </div>

                <CustomPagination
                    current={pagination.current}
                    pageSize={pagination.pageSize}
                    total={pagination.total}
                    onChange={handlePageChange}
                    showSizeChanger
                    showQuickJumper
                    pageSizeOptions={[10, 20, 50, 100]}
                />
            </div>
        </div>
    );
}

export default function RiskPage() {
    return (
        <Suspense fallback={null}>
            <RiskPageContent />
        </Suspense>
    );
}
