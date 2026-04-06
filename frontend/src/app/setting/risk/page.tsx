
'use client';

import '@ant-design/v5-patch-for-react-19';
import React, { Suspense, useEffect, useState } from 'react';
import { Button, Input, Select, DatePicker, Table, Space, message, Modal } from 'antd';
import type { TableColumnsType } from 'antd';
import dayjs from 'dayjs';
import Image from "next/image";
import { useRouter } from 'next/navigation';
import { assets } from "@/assets/assets";
import { useSearchParams } from 'next/navigation';
import { useRiskEditStore } from '@/store/riskEditStore';
import { CustomPagination } from '@/components/table/CustomPagination';

const { RangePicker } = DatePicker;

interface RiskPoint {
    key: string;
    riskId: string;
    riskContent: string;
    applicableContractType: string;
    creator: string;
    updateDate: string;
    status: string;
}

function RiskPageContent() {
    const [riskId, setRiskId] = useState<string>('');
    const [riskContent, setRiskContent] = useState<string>('');
    const [status, setStatus] = useState<string>('');
    const [contractType, setContractType] = useState<string>('');
    const [creator, setCreator] = useState<string>('');
    const [dateRange, setDateRange] = useState<[dayjs.Dayjs, dayjs.Dayjs] | null>(null);
    const [dataSource, setDataSource] = useState<RiskPoint[]>([]);
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

    // 模拟数据
    const mockData: RiskPoint[] = [
        {
            key: '1',
            riskId: '2025121B001',
            riskContent: '这是风险点内容这是风险点内容这是风险点内容这是风险点内容这是风险点内容这是风险点内容这是风险点内容这是风险点内容这是风险点内容这是风险点内容',
            applicableContractType: '货物类合同',
            creator: '平台',
            updateDate: '2025/12/01 00:00:02',
            status: '启用'
        },
        {
            key: '2',
            riskId: '2025121B002',
            riskContent: '这是风险点内容这是风险点内容这是风险点内容这是风险点内容这是风险点内容这是风险点内容这是风险点内容这是风险点内容这是风险点内容这是风险点内容',
            applicableContractType: '货物类合同',
            creator: '平台',
            updateDate: '2025/12/01 00:00:02',
            status: '停用'
        }
    ];

    useEffect(() => {
        // 从URL参数中获取合同类型
        const contractTypeParam = searchParams.get('contractType');
        if (contractTypeParam) {
            setContractType(contractTypeParam);
        }
    }, [searchParams]);

    useEffect(() => {
        fetchRiskList();
    }, [pagination.current, pagination.pageSize, contractType]);

    // 监听 URL 参数变化，当有 refresh 参数时刷新数据
    useEffect(() => {
        const refresh = searchParams.get('refresh');
        if (refresh) {
            setPagination({ current: 1, pageSize: 10, total: 0 });
            fetchRiskList();
        }
    }, [searchParams]);

    // 获取风险点列表
    const fetchRiskList = async () => {
        setLoading(true);
        try {
            // TODO: 调用获取风险点列表接口
            // const response = await getRiskList({
            //     riskId,
            //     riskContent,
            //     status,
            //     creator,
            //     startDate: dateRange?.[0].format('YYYY-MM-DD'),
            //     endDate: dateRange?.[1].format('YYYY-MM-DD'),
            //     page: pagination.current,
            //     pageSize: pagination.pageSize
            // });

            // 模拟数据
            setDataSource(mockData);
            setPagination({
                ...pagination,
                total: mockData.length
            });
        } catch (error) {
            message.error('获取数据失败');
        } finally {
            setLoading(false);
        }
    };

    const handleAdd = () => {
        router.push('/setting/risk/editAndAdd?mode=add');
    };

    // 查询
    const handleSearch = () => {
        setPagination({ ...pagination, current: 1 });
        fetchRiskList();
    };

    // 重置
    const handleReset = () => {
        setRiskId('');
        setRiskContent('');
        setStatus('');
        setContractType('');
        setCreator('');
        setDateRange(null);
        setPagination({ ...pagination, current: 1 });
        fetchRiskList();
    };

    // 编辑
    const handleEdit = (record: RiskPoint) => {
        setRiskData({
            id: record.key,
            riskId: record.riskId,
            contractType: record.applicableContractType,
            applicableScope: 'department',
            riskContent: record.riskContent,
            isEnabled: record.status === '启用' ? 'enabled' : 'disabled'
        });
        router.push(`/setting/risk/editAndAdd?mode=edit&id=${record.riskId}`);
    };

    // 删除
    const handleDelete = (record: RiskPoint) => {
        Modal.confirm({
            title: '删除风险点',
            content: '确认要删除此条数据吗？',
            okText: '确认',
            cancelText: '取消',
            okButtonProps: { danger: true },
            onOk: async () => {
                try {
                    // TODO: 调用删除接口
                    // await deleteRisk(record.riskId);
                    message.success(`删除成功: ${record.riskId}`);
                    await fetchRiskList();
                } catch (error) {
                    message.error('删除失败');
                }
            },
            onCancel() {
                // 取消操作
            },
        });
    };

    // 批量删除
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
                    // TODO: 调用批量删除接口
                    // await batchDeleteRisks(selectedRowKeys);
                    message.success(`批量删除成功: ${selectedRowKeys.length} 项`);
                    setSelectedRowKeys([]);
                    await fetchRiskList();
                } catch (error) {
                    message.error('删除失败');
                }
            },
            onCancel() {
                // 取消操作
            },
        });
    };

    // 分页改变
    const handlePageChange = (page: number, pageSize: number) => {
        setPagination({ ...pagination, current: page, pageSize });
        fetchRiskList();
    };

    const columns: TableColumnsType<RiskPoint> = [
        {
            title: '风险点ID',
            dataIndex: 'riskId',
            key: 'riskId',
            width: 150
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
            width: 100
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
            {/* 搜索区域 */}
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
                            <Select.Option value="启用">启用</Select.Option>
                            <Select.Option value="停用">停用</Select.Option>
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
                            <Select.Option value="货物类合同">货物类合同</Select.Option>
                            <Select.Option value="服务类合同">服务类合同</Select.Option>
                        </Select>
                    </div>
                    <div className="flex items-center">
                        <span className="text-gray-700 w-[6rem] text-left mr-2">创建者：</span>
                        <Select
                            placeholder="请选择"
                            value={creator || undefined}
                            onChange={setCreator}
                            className="flex-1"
                            allowClear
                        >
                            <Select.Option value="平台">平台</Select.Option>
                            <Select.Option value="张三">张三</Select.Option>
                        </Select>
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

            {/* 表格区域 */}
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
                                <Button type="link" style={{ 'color': 'gray' }} size="small" onClick={() => setSelectedRowKeys([])}>
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
                        columns={columns}
                        dataSource={dataSource} 
                        loading={loading}
                        pagination={false}
                    />
                </div>

                {/* 分页区域 - 固定在底部 */}
               
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
