
'use client';

import '@ant-design/v5-patch-for-react-19';
import React, { useState } from 'react';
import { Drawer, Tabs, Table, Button, Input, Space, message } from 'antd';
import { CloseOutlined, EditOutlined } from '@ant-design/icons';
import { assets } from '@/assets/assets';
import Image from 'next/image';
import type { TableColumnsType } from 'antd';
import { useRouter } from 'next/navigation';
import AddMember from './AddMember';

interface Member {
    key: string;
    name: string;
    workId: string;
    department: string;
}

interface RoleDetailDrawerProps {
    open: boolean;
    onClose: () => void;
    roleData?: {
        roleName: string;
        description: string;
        members: Member[];
        permissions: {
            contractReview: string[];
            contractComparison: string[];
            auditLog: string[];
            dataBoard: string[];
        };
    };
}

const RoleDetailDrawer: React.FC<RoleDetailDrawerProps> = ({ open, onClose, roleData }) => {
    const [activeTab, setActiveTab] = useState<string>('members');
    const [searchValue, setSearchValue] = useState('');
    const [addMemberOpen, setAddMemberOpen] = useState(false);
    const router = useRouter();

    // 成员表格列定义
    const memberColumns: TableColumnsType<Member> = [
        {
            title: '姓名',
            dataIndex: 'name',
            key: 'name',
            width: 100
        },
        {
            title: '工号',
            dataIndex: 'workId',
            key: 'workId',
            width: 120
        },
        {
            title: '部门',
            dataIndex: 'department',
            key: 'department',
            width: 120
        },
        {
            title: '操作',
            key: 'action',
            width: 80,
            render: () => (
                <Button type="link" danger size="small" onClick={() => message.info('删除成员')}>
                    移除
                </Button>
            )
        }
    ];

    // 模拟成员数据
    const mockMembers: Member[] = [
        { key: '1', name: '李四', workId: '123456', department: '信息化办' },
        { key: '2', name: '张三', workId: '123456', department: '后勤处' },
        { key: '3', name: '王五', workId: '123456', department: '财务处' },
        { key: '4', name: '用户1', workId: '123456', department: '后勤处' },
        { key: '5', name: '用户2', workId: '123456', department: '后勤处' },
        { key: '6', name: '用户3', workId: '123456', department: '后勤处' }
    ];

    // 权限列表数据
    const permissionsData = {
        contractReview: [
            '发起合同审阅',
            '忽略风险点'
        ],
        contractComparison: [
            '发起合同比对',
            '忽略区别点'
        ],
        auditLog: [
            '查看记录（自己）',
            '查看记录（同部门）',
            '删除记录'
        ],
        dataBoard: [
            '查看个人数据'
        ]
    };

    const handleEditPermissions = () => {
        router.push(`/setting/role/editAndAdd?mode=edit&id=${roleData?.roleName}&section=permissions`);
    };

    const handleAddMember = () => {
        setAddMemberOpen(true);
    };

    const handleAddMemberConfirm = (selectedMembers: Member[]) => {
        // TODO: 处理添加成员后的逻辑
        console.log('添加的成员:', selectedMembers);
        message.success(`已成功添加 ${selectedMembers.length} 个成员`);
    };

    // 成员Tab内容
    const renderMembersTab = () => (
        <div className="space-y-4">
            <div className="flex gap-2">
                <Input
                    placeholder="请输入姓名、部门或工号"
                    value={searchValue}
                    onChange={(e) => setSearchValue(e.target.value)}
                    className="flex-1"
                />
                <Button type="primary" className="bg-blue-500" onClick={handleAddMember}>
                    添加成员
                </Button>
            </div>

            <Table
                columns={memberColumns}
                dataSource={mockMembers}
                pagination={false}
                size="small"
                bordered
            />
        </div>
    );

    // 权限Tab内容
    const renderPermissionsTab = () => (
        <div className="space-y-6">


            {/* 权限统计 */}
            <div className="flex items-center justify-between">
                <span className="text-sm font-semibold">拥有 8 项权限</span>
                <Button type="primary" variant="outlined" size="small" onClick={handleEditPermissions} ghost>
                    编辑
                </Button>
            </div>

            {/* 合同审阅 */}
            <div className="space-y-2">
                <h4 className="font-semibold text-sm">合同审阅</h4>
                <ul className="pl-4 space-y-1 bg-[#fafafa] py-[0.75rem]">
                    {permissionsData.contractReview.map((item, idx) => (
                        <li key={idx} className="flex items-center text-sm text-gray-700">
                            <Image src={assets.YesIcon} alt="yes" width={16} height={16} className="mr-2" />
                            {item}
                        </li>
                    ))}
                </ul>
            </div>

            {/* 合同比对 */}
            <div className="space-y-2">
                <h4 className="font-semibold text-sm">合同比对</h4>
                <ul className="pl-4 space-y-1 bg-[#fafafa] py-[0.75rem]">
                    {permissionsData.contractComparison.map((item, idx) => (
                        <li key={idx} className="flex items-center text-sm text-gray-700">
                            <Image src={assets.YesIcon} alt="yes" width={16} height={16} className="mr-2" />
                            {item}
                        </li>
                    ))}
                </ul>
            </div>

            {/* 智审记录 */}
            <div className="space-y-2">
                <h4 className="font-semibold text-sm">智审记录</h4>
                <ul className="pl-4 space-y-1 bg-[#fafafa] py-[0.75rem]">
                    {permissionsData.auditLog.map((item, idx) => (
                        <li key={idx} className="flex items-center text-sm text-gray-700">
                            <Image src={assets.YesIcon} alt="yes" width={16} height={16} className="mr-2" />
                            {item}
                        </li>
                    ))}
                </ul>
            </div>

            {/* 数据看板 */}
            <div className="space-y-2">
                <h4 className="font-semibold text-sm">数据看板</h4>
                <ul className="pl-4 space-y-1 bg-[#fafafa] py-[0.75rem]">
                    {permissionsData.dataBoard.map((item, idx) => (
                        <li key={idx} className="flex items-center text-sm text-gray-700">
                            <Image src={assets.YesIcon} alt="yes" width={16} height={16} className="mr-2" />
                            {item}
                        </li>
                    ))}
                </ul>
            </div>
        </div>
    );

    const handleEditRole = () => {
        router.push(`/setting/role/editAndAdd?mode=edit&id=${roleData?.roleName}`);
    };

    return (
        <>
            <Drawer
                title="角色详情"
                placement="right"
                onClose={onClose}
                open={open}
                width={500}
                closeIcon={<CloseOutlined />}
            >
                {/* 角色名称和描述 */}
                <div className="pb-4">
                    <div className="flex items-center gap-2 mb-2">
                        <h3 className="text-base font-semibold">{roleData?.roleName}</h3>
                        <EditOutlined 
                            className="text-blue-500 cursor-pointer text-sm" 
                            onClick={handleEditRole}
                        />
                    </div>
                    <p className="text-sm text-gray-600">
                        {roleData?.description}
                    </p>
                </div>
                <Tabs
                    activeKey={activeTab}
                    onChange={setActiveTab}
                    items={[
                        {
                            key: 'members',
                            label: '成员',
                            children: renderMembersTab()
                        },
                        {
                            key: 'permissions',
                            label: '权限',
                            children: renderPermissionsTab()
                        }
                    ]}
                />
            </Drawer>

            <AddMember
                open={addMemberOpen}
                onClose={() => setAddMemberOpen(false)}
                roleName={roleData?.roleName || ''}
                onConfirm={handleAddMemberConfirm}
            />
        </>
    );
};

export default RoleDetailDrawer;
