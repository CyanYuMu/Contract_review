'use client';

import '@ant-design/v5-patch-for-react-19';
import React, { useState, useEffect } from 'react';
import { Button, Input, Select, Table, Space, message, Modal, Switch } from 'antd';
import type { TableColumnsType } from 'antd';
import { useRouter } from 'next/navigation';
import { CustomPagination } from '@/components/table/CustomPagination';
import Image from 'next/image';
import { assets } from '@/assets/assets';

interface PermissionConfig {
    key: string;
    permissionId: string;
    permissionName: string;
    permissionDesc: string;
    module: string;
    status: boolean;
}

export default function PermissionPage() {
    const [searchPermissionId, setSearchPermissionId] = useState<string>('');
    const [searchPermissionName, setSearchPermissionName] = useState<string>('');
    const [searchModule, setSearchModule] = useState<string | undefined>(undefined);
    const [searchStatus, setSearchStatus] = useState<string | undefined>(undefined);
    const [dataSource, setDataSource] = useState<PermissionConfig[]>([]);
    const [loading, setLoading] = useState<boolean>(false);
    const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);
    const [pagination, setPagination] = useState({
        current: 1,
        pageSize: 10,
        total: 0
    });
    const router = useRouter();

    // 所属模块选项
    const moduleOptions = [
        { label: '角色配置', value: '角色配置' },
        { label: '模块1', value: '模块1' },
        { label: '模块2', value: '模块2' },
        { label: '模块3', value: '模块3' },
        { label: '模块4', value: '模块4' },
        { label: '模块5', value: '模块5' },
    ];

    // 权限状态选项
    const statusOptions = [
        { label: '启用', value: 'enabled' },
        { label: '停用', value: 'disabled' },
    ];

    // 模拟数据
    const mockData: PermissionConfig[] = [
        {
            key: '1',
            permissionId: '123455',
            permissionName: '管理角色内用户',
            permissionDesc: '允许该角色访问角色配置页面，查看所有角色内的用户，并新增或移除用户',
            module: '角色配置',
            status: true
        },
        {
            key: '2',
            permissionId: '123456',
            permissionName: '修改角色信息',
            permissionDesc: '允许该角色访问角色配置页面，查看所有角色信息，并可编辑角色基础信息（...',
            module: '角色配置',
            status: true
        },
        {
            key: '3',
            permissionId: '102124',
            permissionName: '编辑角色权限',
            permissionDesc: '允许该角色访问角色配置页面，查看所有角色信息，并可编辑角色权限',
            module: '角色配置',
            status: true
        },
        {
            key: '4',
            permissionId: '12355',
            permissionName: '删除角色',
            permissionDesc: '允许该角色访问角色配置页面，查看所有角色，并可删除角色（默认的三个角...',
            module: '角色配置',
            status: true
        },
        {
            key: '5',
            permissionId: '123e55',
            permissionName: '新建角色',
            permissionDesc: '允许该角色访问角色配置页面，并可新建角色，为新建角色设置角色信息和配...',
            module: '角色配置',
            status: true
        },
        {
            key: '6',
            permissionId: '2143re',
            permissionName: '权限名称1',
            permissionDesc: '这是权限描述，方便其他管理员了解该权限详细内容',
            module: '模块1',
            status: true
        },
        {
            key: '7',
            permissionId: 'rewr2131',
            permissionName: '权限名称2',
            permissionDesc: '这是权限描述，方便其他管理员了解该权限详细内容',
            module: '模块2',
            status: true
        },
        {
            key: '8',
            permissionId: '435dgsd',
            permissionName: '权限名称3',
            permissionDesc: '这是权限描述，方便其他管理员了解该权限详细内容',
            module: '模块3',
            status: false
        },
        {
            key: '9',
            permissionId: '112eet32',
            permissionName: '权限名称4',
            permissionDesc: '这是权限描述，方便其他管理员了解该权限详细内容',
            module: '模块4',
            status: true
        },
        {
            key: '10',
            permissionId: '3dasffwe',
            permissionName: '权限名称5',
            permissionDesc: '这是权限描述，方便其他管理员了解该权限详细内容',
            module: '模块5',
            status: true
        },
    ];

    useEffect(() => {
        fetchPermissionList();
    }, []);

    // 获取权限列表
    const fetchPermissionList = async () => {
        setLoading(true);
        try {
            // TODO: 调用获取权限列表接口
            // const response = await getPermissionList({ ... });

            // 模拟数据
            setDataSource(mockData);
            setPagination({
                ...pagination,
                total: mockData.length * 9 // 模拟多页数据
            });
        } catch (error) {
            message.error('获取数据失败');
        } finally {
            setLoading(false);
        }
    };

    // 重置
    const handleReset = () => {
        setSearchPermissionId('');
        setSearchPermissionName('');
        setSearchModule(undefined);
        setSearchStatus(undefined);
        setPagination({ ...pagination, current: 1 });
        fetchPermissionList();
    };

    // 查询
    const handleSearch = () => {
        setPagination({ ...pagination, current: 1 });
        fetchPermissionList();
    };

    // 新增权限
    const handleAdd = () => {
        router.push('/setting/permission/editAndAdd?mode=add');
    };

    // 编辑权限
    const handleEdit = (record: PermissionConfig) => {
        router.push(`/setting/permission/editAndAdd?mode=edit&id=${record.permissionId}`);
    };

    // 查看分配角色
    const handleViewRoles = (record: PermissionConfig) => {
        message.info(`查看 ${record.permissionName} 的分配角色`);
    };

    // 删除权限
    const handleDelete = (record: PermissionConfig) => {
        Modal.confirm({
            title: '删除权限',
            content: `确定要删除权限 "${record.permissionName}" 吗？`,
            okText: '确定',
            cancelText: '取消',
            okButtonProps: { danger: true },
            onOk() {
                // TODO: 调用删除接口
                message.success(`已删除权限: ${record.permissionName}`);
                fetchPermissionList();
            },
        });
    };

    // 批量删除
    const handleBatchDelete = () => {
        if (selectedRowKeys.length === 0) {
            message.warning('请先选择要删除的权限');
            return;
        }
        Modal.confirm({
            title: '批量删除权限',
            content: `确定要删除选中的 ${selectedRowKeys.length} 个权限吗？`,
            okText: '确定',
            cancelText: '取消',
            okButtonProps: { danger: true },
            onOk() {
                // TODO: 调用批量删除接口
                message.success(`已删除 ${selectedRowKeys.length} 个权限`);
                setSelectedRowKeys([]);
                fetchPermissionList();
            },
        });
    };

    // 取消选择
    const handleCancelSelection = () => {
        setSelectedRowKeys([]);
    };

    // 切换权限状态
    const handleStatusChange = (checked: boolean, record: PermissionConfig) => {
        // TODO: 调用更新状态接口
        const newDataSource = dataSource.map(item => {
            if (item.key === record.key) {
                return { ...item, status: checked };
            }
            return item;
        });
        setDataSource(newDataSource);
        message.success(`权限 "${record.permissionName}" 已${checked ? '启用' : '停用'}`);
    };

    // 分页改变
    const handlePageChange = (page: number, pageSize: number) => {
        setPagination({ ...pagination, current: page, pageSize });
        fetchPermissionList();
    };

    // 行选择配置
    const rowSelection = {
        selectedRowKeys,
        onChange: (newSelectedRowKeys: React.Key[]) => {
            setSelectedRowKeys(newSelectedRowKeys);
        },
    };

    const columns: TableColumnsType<PermissionConfig> = [
        {
            title: '权限ID',
            dataIndex: 'permissionId',
            key: 'permissionId',
            width: 120,
        },
        {
            title: '权限名称',
            dataIndex: 'permissionName',
            key: 'permissionName',
            width: 140,
        },
        {
            title: '权限描述',
            dataIndex: 'permissionDesc',
            key: 'permissionDesc',
            ellipsis: true,
        },
        {
            title: '所属模块',
            dataIndex: 'module',
            key: 'module',
            width: 120,
        },
        {
            title: '状态',
            dataIndex: 'status',
            key: 'status',
            width: 140,
            render: (status: boolean, record) => (
                <div className="flex items-center gap-2">
                    <Switch
                        checked={status}
                        onChange={(checked) => handleStatusChange(checked, record)}
                    />
                    <span className={status ? 'text-gray-700' : 'text-gray-400'}>
                        {status ? '启用' : '停用'}
                    </span>
                </div>
            ),
        },
        {
            title: '操作',
            key: 'action',
            width: 200,
            render: (_, record) => (
                <Space size="middle">
                    <Button
                        type="link"
                        size="small"
                        className="!p-0"
                        onClick={() => handleEdit(record)}
                    >
                        编辑
                    </Button>
                    <Button
                        type="link"
                        size="small"
                        className="!p-0"
                        onClick={() => handleViewRoles(record)}
                    >
                        查看分配角色
                    </Button>
                    <Button
                        type="link"
                        danger
                        size="small"
                        className="!p-0"
                        onClick={() => handleDelete(record)}
                    >
                        删除
                    </Button>
                </Space>
            ),
        },
    ];

    return (
        <div className="flex flex-col bg-[#f1f1f1] h-[100%]">
            {/* 搜索区域 */}
            <div className="mb-4 bg-white p-[1rem] rounded">
                <div className="grid grid-cols-4 gap-x-4 gap-y-4">
                    <div className="flex items-center">
                        <span className="text-gray-700 w-[5rem] text-left mr-2">权限ID：</span>
                        <Input
                            placeholder="请输入权限ID"
                            value={searchPermissionId}
                            onChange={(e) => setSearchPermissionId(e.target.value)}
                            className="flex-1"
                            onPressEnter={handleSearch}
                        />
                    </div>
                    <div className="flex items-center">
                        <span className="text-gray-700 w-[5rem] text-left mr-2">权限名称：</span>
                        <Input
                            placeholder="请输入权限名称"
                            value={searchPermissionName}
                            onChange={(e) => setSearchPermissionName(e.target.value)}
                            className="flex-1"
                            onPressEnter={handleSearch}
                        />
                    </div>
                    <div className="flex items-center">
                        <span className="text-gray-700 w-[5rem] text-left mr-2">所属模块：</span>
                        <Select
                            placeholder="请选择"
                            value={searchModule}
                            onChange={setSearchModule}
                            options={moduleOptions}
                            allowClear
                            className="flex-1"
                        />
                    </div>
                    <div className="flex items-center">
                        <span className="text-gray-700 w-[5rem] text-left mr-2">权限状态：</span>
                        <Select
                            placeholder="请选择"
                            value={searchStatus}
                            onChange={setSearchStatus}
                            options={statusOptions}
                            allowClear
                            className="flex-1"
                        />
                    </div>
                </div>
                <div className="flex justify-end gap-2 mt-4">
                    <Button onClick={handleReset}>重 置</Button>
                    <Button type="primary" onClick={handleSearch}>查询</Button>
                </div>
            </div>

            {/* 表格区域 */}
            <div className="bg-white flex-1 flex flex-col overflow-hidden p-[1rem] relative pb-0 rounded">
                <div className="mb-4 flex items-center justify-between">
                    <div className="text-[1.25rem] text-black font-bold">
                        权限列表
                    </div>
                    <Button type="primary" onClick={handleAdd}>
                        新增权限
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
                                <Button type="link" style={{ 'color': 'gray' }} size="small" onClick={handleCancelSelection}>
                                    取消选择
                                </Button>
                                <Button type="link" size="small" onClick={handleBatchDelete}>
                                    批量删除
                                </Button>
                            </span>
                        </div>
                    </div>
                )}

                {/* 表格 */}
                <div className="flex-1 overflow-y-auto">
                    <Table
                        rowSelection={rowSelection}
                        columns={columns}
                        dataSource={dataSource}
                        loading={loading}
                        pagination={false}
                    />
                </div>

                {/* 分页区域 */}
                <CustomPagination
                    current={pagination.current}
                    pageSize={pagination.pageSize}
                    total={pagination.total}
                    onChange={handlePageChange}
                    showQuickJumper={false}
                />
            </div>
        </div>
    );
}
