
'use client';

import React, { useState } from 'react';
import { Modal, Input, Table, Button, message } from 'antd';
import type { TableColumnsType } from 'antd';
import { SearchOutlined } from '@ant-design/icons';

interface Member {
    key: string;
    name: string;
    workId: string;
    department: string;
}

interface AddMemberProps {
    open: boolean;
    onClose: () => void;
    roleName: string;
    onConfirm?: (selectedMembers: Member[]) => void;
}

export default function AddMember({ open, onClose, roleName, onConfirm }: AddMemberProps) {
    const [searchValue, setSearchValue] = useState<string>('');
    const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);
    const [loading, setLoading] = useState(false);

    // 模拟成员数据
    const mockMembers: Member[] = [
        { key: '1', name: '李四', workId: '123456', department: '信息化办' },
        { key: '2', name: '张三', workId: '123456', department: '信息化办' },
        { key: '3', name: '王五', workId: '123456', department: '后勤处' },
        { key: '4', name: '李四', workId: '123456', department: '财务处' },
        { key: '5', name: '李四', workId: '123456', department: '部门1' },
        { key: '6', name: '李四', workId: '123456', department: '部门2' },
        { key: '7', name: '李四', workId: '123456', department: '部门2' }
    ];

    // 过滤数据
    const filteredData = mockMembers.filter(item =>
        item.name.toLowerCase().includes(searchValue.toLowerCase()) ||
        item.workId.includes(searchValue) ||
        item.department.toLowerCase().includes(searchValue.toLowerCase())
    );

    // 已选中的成员数据
    const selectedMembers = mockMembers.filter(item =>
        selectedRowKeys.includes(item.key)
    );

    const leftColumns: TableColumnsType<Member> = [
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
        }
    ];

    const rightColumns: TableColumnsType<Member> = [
        {
            title: '姓名',
            dataIndex: 'name',
            key: 'name',
            width:'25%'
        },
        {
            title: '工号',
            dataIndex: 'workId',
            key: 'workId',
            width:'25%'
        },
        {
            title: '部门',
            dataIndex: 'department',
            key: 'department',
            width:'25%'
        },
        {
            title: '操作',
            key: 'action',
            width:'25%',
            render: (_, record) => (
                <Button
                    type="link"
                    size="small"
                    danger
                    onClick={() => {
                        setSelectedRowKeys(
                            selectedRowKeys.filter(key => key !== record.key)
                        );
                    }}
                >
                    移除
                </Button>
            )
        }
    ];

    const handleConfirm = async () => {
        if (selectedRowKeys.length === 0) {
            message.warning('请选择要添加的成员');
            return;
        }

        try {
            setLoading(true);

            // TODO: 调用添加成员接口
            // await addMembersToRole(roleName, selectedMembers);

            message.success(`已成功添加 ${selectedRowKeys.length} 个成员到角色 "${roleName}"`);
            onConfirm?.(selectedMembers);
            handleClose();
        } catch (error) {
            message.error('添加成员失败，请稍后重试');
        } finally {
            setLoading(false);
        }
    };

    const handleClose = () => {
        setSearchValue('');
        setSelectedRowKeys([]);
        onClose();
    };

    const handleClearSelection = () => {
        setSelectedRowKeys([]);
    };

    const rowSelection = {
        selectedRowKeys,
        onChange: (newSelectedRowKeys: React.Key[]) => {
            setSelectedRowKeys(newSelectedRowKeys);
        }
    };

    return (
        <Modal
            title={
                <div className='text-[1.25rem] text-black font-bold border-l-[0.31rem] border-[#2260F2] pl-[0.75rem] mb-4'>
                    添加成员
                </div>
            }
            open={open}
            onCancel={handleClose}
            width={1000}
            footer={[
                <Button key="cancel" onClick={handleClose}>
                    取消
                </Button>,
                <Button
                    key="submit"
                    type="primary"
                    loading={loading}
                    onClick={handleConfirm}
                >
                    确认添加
                </Button>
            ]}
        >
            <div className="flex gap-6">
                {/* 左侧：成员列表 */}
                <div className="flex-1 min-w-0">
                    <div className="mb-4">
                        <Input
                            placeholder="请输入人要添加用户的姓名或工号"
                            prefix={<SearchOutlined />}
                            value={searchValue}
                            onChange={(e) => setSearchValue(e.target.value)}
                            size="large"
                        />
                    </div>

                    <Table<Member>
                        columns={leftColumns}
                        dataSource={filteredData}
                        pagination={false}
                        rowSelection={rowSelection}
                        size="small"
                        scroll={{ y: 300, x: '100%' }}
                        bordered
                        style={{ minHeight: '300px' }}
                    />

                    <div className="mt-3 text-center">
                        <Button type="link" size="small">
                            点击查看更多
                        </Button>
                    </div>
                </div>

                {/* 右侧：已选择成员 */}
                <div className="flex-1 min-w-0">
                    <div className="flex items-center justify-between mb-4">
                        <span className="font-medium">已选中 {selectedRowKeys.length} 名用户</span>
                        <Button
                            type="link"
                            size="small"
                            onClick={handleClearSelection}
                        >
                            清空选择
                        </Button>
                    </div>

                    <Table<Member>
                        columns={rightColumns}
                        dataSource={selectedMembers}
                        pagination={false}
                        size="small"
                        scroll={selectedMembers.length > 4 ? { y: 300, x: '100%' } : { x: '100%' }}
                        bordered
                        style={{ minHeight: '300px' }}
                    />
                </div>
            </div>
        </Modal>
    );
}