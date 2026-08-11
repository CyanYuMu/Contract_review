'use client';

import React, { useState, useEffect } from 'react';
import { Button, Input, Select, DatePicker, Table, Space, message, Modal } from 'antd';
import { PlusCircleOutlined } from '@ant-design/icons';
import type { TableColumnsType } from 'antd';
import dayjs from 'dayjs';
import Image from "next/image";
import { useRouter } from 'next/navigation';
import { assets } from "@/assets/assets";
import { useContractTypeEditStore } from '@/store/contractTypeEditStore';
import { CustomPagination } from '@/components/table/CustomPagination';
import {
    getContractTypePage,
    getContractTypeCreators,
    deleteContractType,
    batchDeleteContractType,
    ContractTypeCreator,
    ContractTypeListItem
} from '@/lib/api/contractType';

const { RangePicker } = DatePicker;

interface PromptTemplate {
    key: string;
    id?: string;
    contractTypeName: string;
    templateContent: string;
    creator: string;
    updateDate: string;
}

export default function PromptPage() {
    const [contractTypeName, setContractTypeName] = useState<string>('');
    const [creator, setCreator] = useState<string>('');
    const [dateRange, setDateRange] = useState<[dayjs.Dayjs, dayjs.Dayjs] | null>(null);
    const [dataSource, setDataSource] = useState<PromptTemplate[]>([]);
    const [loading, setLoading] = useState<boolean>(false);
    const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);
    const [creators, setCreators] = useState<ContractTypeCreator[]>([]);
    const [pagination, setPagination] = useState({
        current: 1,
        pageSize: 10,
        total: 0
    });

    const router = useRouter();
    const setContractTypeData = useContractTypeEditStore((state) => state.setContractTypeData);

    useEffect(() => {
        fetchCreators();
        fetchPromptList();
    }, []);

    // 获取创建者列表
    const fetchCreators = async () => {
        try {
            const response = await getContractTypeCreators();
            if (response?.code === 200 && response?.data) {
                setCreators(response.data);
            }
        } catch (error) {
            console.error('获取创建者列表失败:', error);
        }
    };

    // 获取合同类型列表
    const fetchPromptList = async (page = pagination.current, pageSize = pagination.pageSize) => {
        setLoading(true);
        try {
            const response = await getContractTypePage({
                contractTypeName: contractTypeName || undefined,
                creator: creator || undefined,
                startDate: dateRange?.[0]?.format('YYYY-MM-DD'),
                endDate: dateRange?.[1]?.format('YYYY-MM-DD'),
                page,
                pageSize
            });

            if (response?.code === 200 && response?.data) {
                const list = (response.data.list || []).map((item: ContractTypeListItem) => ({
                    ...item,
                    key: item.id
                }));
                setDataSource(list);
                setPagination({
                    current: response.data.page || page,
                    pageSize: response.data.pageSize || pageSize,
                    total: response.data.total || 0
                });
            } else {
                setDataSource([]);
                setPagination({ ...pagination, total: 0 });
            }
        } catch (error) {
            message.error('获取数据失败');
            setDataSource([]);
        } finally {
            setLoading(false);
        }
    };

    // 查询
    const handleSearch = () => {
        fetchPromptList(1, pagination.pageSize);
    };

    // 重置
    const handleReset = () => {
        setContractTypeName('');
        setCreator('');
        setDateRange(null);
        // 需要在状态更新后再请求，这里直接请求不带筛选条件的数据
        setTimeout(() => {
            fetchPromptList(1, pagination.pageSize);
        }, 0);
    };

    // 分页改变
    const handlePageChange = (page: number, pageSize: number) => {
        fetchPromptList(page, pageSize);
    };

    // 新增类型
    const handleAdd = () => {
        setContractTypeData({});
        router.push('/setting/contractType/editAndAdd?mode=add');
    };

    // 编辑
    const handleEdit = (record: PromptTemplate) => {
        setContractTypeData({
            id: record.id,
            contractTypeName: record.contractTypeName,
            templateContent: record.templateContent
        });
        router.push(`/setting/contractType/editAndAdd?mode=edit&id=${record.id}`);
    };

    // 查看风险点
    const handleViewRisks = (record: PromptTemplate) => {
        router.push(`/setting/risk?contractType=${encodeURIComponent(record.contractTypeName)}`);
    };

    // 删除
    const handleDelete = (record: PromptTemplate) => {
        Modal.confirm({
            title: '删除合同类型',
            content: '确认要删除此条数据吗？',
            okText: '确认',
            cancelText: '取消',
            okButtonProps: { danger: true },
            onOk: async () => {
                try {
                    const response = await deleteContractType(Number(record.id));
                    if (response?.code === 200) {
                        message.success(`删除成功: ${record.contractTypeName}`);
                        await fetchPromptList();
                    } else {
                        message.error(response?.msg || '删除失败');
                    }
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
            title: '批量删除合同类型',
            content: `确认要删除选中的 ${selectedRowKeys.length} 项吗？`,
            okText: '确认',
            cancelText: '取消',
            okButtonProps: { danger: true },
            onOk: async () => {
                try {
                    const ids = selectedRowKeys.map(key => Number(key));
                    const response = await batchDeleteContractType(ids);
                    if (response?.code === 200) {
                        message.success(`批量删除成功: ${selectedRowKeys.length} 项`);
                        setSelectedRowKeys([]);
                        await fetchPromptList();
                    } else {
                        message.error(response?.msg || '删除失败');
                    }
                } catch (error) {
                    message.error('删除失败');
                }
            },
            onCancel() {
                // 取消操作
            },
        });
    };

    const columns: TableColumnsType<PromptTemplate> = [
        {
            title: '合同类型名称',
            dataIndex: 'contractTypeName',
            key: 'contractTypeName',
            width: 150
        },
        {
            title: '提示词模板',
            dataIndex: 'templateContent',
            key: 'templateContent',
            ellipsis: true
        },
        {
            title: '创建人',
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
            title: '操作',
            key: 'action',
            width: 250,
            render: (_, record) => (
                <Space size="small">
                    <Button type="link" size="small" onClick={() => handleEdit(record)}>
                        编辑
                    </Button>
                    <Button type="link" size="small" onClick={() => handleViewRisks(record)}>
                        查看风险点
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
                <div className="grid grid-cols-4 gap-4">
                    <div className="flex items-center">
                        <span className="text-gray-700 mr-2 whitespace-nowrap">合同类型名称：</span>
                        <Input
                            placeholder="请输入合同类型名称"
                            value={contractTypeName}
                            onChange={(e) => setContractTypeName(e.target.value)}
                            className="flex-1"
                        />
                    </div>
                    <div className="flex items-center">
                        <span className="text-gray-700 mr-2 whitespace-nowrap">创建人：</span>
                        <Select
                            placeholder="请选择"
                            value={creator || undefined}
                            onChange={setCreator}
                            className="flex-1"
                            allowClear
                        >
                            {creators.map((item) => (
                                <Select.Option key={item.id} value={item.name}>
                                    {item.name}
                                </Select.Option>
                            ))}
                        </Select>
                    </div>
                    <div className="flex items-center ">
                        <span className="text-gray-700 mr-2 whitespace-nowrap">更改日期：</span>
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
                        合同类型列表
                    </div>
                    <Button type="primary" onClick={handleAdd} icon={<PlusCircleOutlined />}>
                        新增合同类型
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

                {/* 分页区域 - 使用自定义分页组件 */}
                <CustomPagination
                    current={pagination.current}
                    pageSize={pagination.pageSize}
                    total={pagination.total}
                    onChange={handlePageChange}
                />
            </div>
        </div>
    );
}