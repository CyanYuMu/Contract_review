
'use client';

import '@ant-design/v5-patch-for-react-19';
import React, { useState, useEffect } from 'react';
import { Button, Input, Select, DatePicker, Table, Space, message, Modal } from 'antd';
import { PlusCircleOutlined } from '@ant-design/icons';
import type { TableColumnsType } from 'antd';
import RoleDetailDrawer from './components/Detail';
import AddMember from './components/AddMember';
import { useRouter } from 'next/navigation';
import { CustomPagination } from '@/components/table/CustomPagination';

interface RoleConfig {
    key: string;
    roleName: string;
    members: string;
    permissions: string;
}

export default function RolePage() {
    const [searchType, setSearchType] = useState<string>('role');
    const [searchValue, setSearchValue] = useState<string>('');
    const [dataSource, setDataSource] = useState<RoleConfig[]>([]);
    const [loading, setLoading] = useState<boolean>(false);
    const [pagination, setPagination] = useState({
        current: 1,
        pageSize: 10,
        total: 0
    });
    const [detailDrawerOpen, setDetailDrawerOpen] = useState(false);
    const [selectedRole, setSelectedRole] = useState<RoleConfig | null>(null);
    const [addMemberOpen, setAddMemberOpen] = useState(false);
    const [selectedRoleForAddMember, setSelectedRoleForAddMember] = useState<RoleConfig | null>(null);
    const router = useRouter();

    // 搜索类型选项
    const searchTypeOptions = [
        { label: '角色', value: 'role' },
        { label: '成员姓名', value: 'member' },
        { label: '权限', value: 'permission' }
    ];

    // 模拟数据
    const mockData: RoleConfig[] = [
        {
            key: '1',
            roleName: '超级管理员',
            members: '法三、学四、五五、法三、学四、五五、学四、五五、法三、学四、法三等12人',
            permissions: '全部权限'
        },
        {
            key: '2',
            roleName: '部门管理员',
            members: '法三',
            permissions: '部门权限'
        },
        {
            key: '3',
            roleName: '普通用户',
            members: '法三、学四',
            permissions: '发起合同咨询、发起合同比对、查看审核记录 等12项'
        },
        {
            key: '4',
            roleName: '角色1',
            members: '法三、学四、五五',
            permissions: '访问所有预览模板'
        },
        {
            key: '5',
            roleName: '角色2',
            members: '法三、学四、五五、法三',
            permissions: '访问平台数据看板'
        },
        {
            key: '6',
            roleName: '角色3',
            members: '法三、学四、五五、法三',
            permissions: '访问所有记录'
        }
    ];

    useEffect(() => {
        fetchRoleList();
    }, []);

    // 获取角色列表
    const fetchRoleList = async () => {
        setLoading(true);
        try {
            // TODO: 调用获取角色列表接口
            // const response = await getRoleList({ 
            //     searchType, 
            //     searchValue 
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

    // 重置
    const handleReset = () => {
        setSearchType('role');
        setPagination({ ...pagination, current: 1 });
        setSearchValue('');
        fetchRoleList();
    };
    
    const handleSelectChange = (e: any) => {
        console.log(e);
        setSearchType(e)
    }

    // 新增角色
    const handleAdd = () => {
        router.push('/setting/role/editAndAdd?mode=add');
    };

    // 管理员日志
    const handleAdminLog = () => {
        message.info('查看管理员日志');
    };

    // 详情
    const handleDetail = (record: RoleConfig) => {
        setSelectedRole(record);
        setDetailDrawerOpen(true);
    };

    // 查看成员
    const handleViewMembers = (record: RoleConfig) => {
        setSelectedRole(record);
        setDetailDrawerOpen(true);
    };

    // 添加成员
    const handleAddMembers = (record: RoleConfig) => {
        setSelectedRoleForAddMember(record);
        setAddMemberOpen(true);
    };

    // 删除角色
    const handleDelete = (record: RoleConfig) => {
        Modal.confirm({
            title: '删除角色',
            content: `确定要删除角色 "${record.roleName}" 吗？`,
            okText: '确定',
            cancelText: '取消',
            okButtonProps: { danger: true },
            onOk() {
                // TODO: 调用删除接口
                // await deleteRole(record.key);
                message.success(`已删除角色: ${record.roleName}`);
            },
            onCancel() {
                console.log('取消删除');
            }
        });
    };

    // 获取输入框占位符
    const getPlaceholder = () => {
        switch (searchType) {
            case 'role':
                return '请输入角色名称';
            case 'member':
                return '请输入成员姓名';
            case 'permission':
                return '请输入权限';
            default:
                return '请输入';
        }
    };
    
    // 查询
    const handleSearch = () => {
        setPagination({ ...pagination, current: 1 });
        fetchRoleList();
    };

    // 分页改变
    const handlePageChange = (page: number, pageSize: number) => {
        setPagination({ ...pagination, current: page, pageSize });
        fetchRoleList();
    };
    
    const columns: TableColumnsType<RoleConfig> = [
        {
            title: <span className="pl-6">角色名称</span>,
            dataIndex: 'roleName',
            key: 'roleName',
            width: 150,
            render: (text) => (
                <span className="pl-6">{text}</span>
            )
        },
        {
            title: '成员',
            dataIndex: 'members',
            key: 'members',
            ellipsis: true
        },
        {
            title: '权限',
            dataIndex: 'permissions',
            key: 'permissions',
            width: 350
        },
        {
            title: '操作',
            key: 'action',
            width: 260,
            render: (_, record) => (
                <Space size="middle" onClick={(e) => e.stopPropagation()}>
                    <Button
                        type="link"
                        size="small"
                        className="!p-0"
                        onClick={() => handleViewMembers(record)}
                    >
                        查看成员
                    </Button>
                    <Button
                        type="link"
                        size="small"
                        className="!p-0"
                        onClick={() => handleAddMembers(record)}
                    >
                        添加成员
                    </Button>
                    <Button
                        type="link"
                        danger
                        size="small"
                        className="!p-0"
                        onClick={() => handleDelete(record)}
                    >
                        删除角色
                    </Button>
                </Space>
            )
        }
    ];

    return (
        <div className="flex flex-col bg-[#f1f1f1] h-[100%]">
            {/* 搜索区域 */}
            <div className="bg-white p-[1rem]">
                <div className="flex justify-between items-center mb-4">
                    <h2 className="m-0 text-lg font-bold text-black">角色配置</h2>
                </div>

                <div className="flex items-center">
                    <Space.Compact>
                        <Select
                            value={searchType}
                            onChange={handleSelectChange}
                            options={searchTypeOptions}
                            className="!w-[100]"
                        />
                        <Input
                            placeholder={getPlaceholder()}
                            value={searchValue}
                            onChange={(e) => setSearchValue(e.target.value)}
                            className="!w-[16rem]"
                            onPressEnter={handleSearch}
                        />
                    </Space.Compact>

                    <div className="flex gap-2 ml-auto">
                        <Button onClick={handleAdminLog}>管理员日志</Button>
                        <Button type="primary" onClick={handleAdd} icon={<PlusCircleOutlined />}>
                            新增角色
                        </Button>
                    </div>
                </div>
            </div>

            {/* 表格区域 */}
            <div className="bg-white flex-1 flex flex-col overflow-hidden p-[1rem] relative pb-0">
                <div className="flex-1 overflow-auto">
                    <Table
                        columns={columns}
                        dataSource={dataSource}
                        loading={loading}
                        pagination={false}
                        onRow={(record) => ({
                            onClick: () => handleDetail(record),
                            style: { cursor: 'pointer' }
                        })}
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

            <RoleDetailDrawer
                open={detailDrawerOpen}
                onClose={() => {
                    setDetailDrawerOpen(false);
                    setSelectedRole(null);
                }}
                roleData={selectedRole ? {
                    roleName: selectedRole.roleName,
                    description: '这是角色说明，管理员可以添加角色说明以向其他管理员说明该角色属性。',
                    members: [],
                    permissions: {
                        contractReview: [],
                        contractComparison: [],
                        auditLog: [],
                        dataBoard: []
                    }
                } : undefined}
            />
            
            <AddMember
                open={addMemberOpen}
                onClose={() => {
                    setAddMemberOpen(false);
                    setSelectedRoleForAddMember(null);
                }}
                roleName={selectedRoleForAddMember?.roleName || ''}
                onConfirm={(selectedMembers) => {
                    // TODO: 处理添加成员后的逻辑
                    console.log('添加的成员:', selectedMembers);
                }}
            />
        </div>
    );
}
