
'use client';

import '@ant-design/v5-patch-for-react-19';
import React, { useEffect, useCallback, useState, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { Form, Input, Button, Checkbox, message, Space, Modal } from 'antd';
import type { CheckboxChangeEvent } from 'antd/es/checkbox';
import { InfoCircleOutlined } from '@ant-design/icons';

interface RoleFormData {
    roleName: string;
    roleDescription: string;
    permissions: Record<string, boolean>;
}

interface PermissionItem {
    key: string;
    label: string;
}

interface PermissionSection {
    title: string;
    items: PermissionItem[];
}

function RoleEditAndAddPage() {
    const [form] = Form.useForm<RoleFormData>();
    const router = useRouter();
    const searchParams = useSearchParams();
    const mode = (searchParams.get('mode') || 'add') as 'add' | 'edit';
    const roleId = searchParams.get('id');
    const section = searchParams.get('section');
    const [loading, setLoading] = useState(false);
    const [selectedCount, setSelectedCount] = useState(0);

    const title = mode === 'add' ? '新增角色' : '编辑角色';

    // 权限配置
    const permissionSections: PermissionSection[] = [
        {
            title: '合同审阅',
            items: [
                { key: 'contractReview', label: '发起合同审阅' },
                { key: 'riskIdentification', label: '查看风险点' },
                { key: 'contractModification', label: '修订风险点' },
                { key: 'contractExport', label: '导出修订后文件' },
                { key: 'contractSwitching', label: '切换审阅状态' }
            ]
        },
        {
            title: '合同比对',
            items: [
                { key: 'contractComparison', label: '发起合同比对' },
                { key: 'riskClassification', label: '查看风险区别点' },
                { key: 'contractExportAnnotation', label: '导出标注文件' },
                { key: 'contractSwitchingStatus', label: '切换审核状态' }
            ]
        },
        {
            title: '智审记录',
            items: [
                { key: 'auditRecordSelf', label: '查看记录（自己）' },
                { key: 'auditRecordDepartment', label: '查看记录（同部门）' },
                { key: 'auditRecordPlatform', label: '查看记录（全平台）' },
                { key: 'auditRecordOthers', label: '查看记录（他人）' }
            ]
        },
        {
            title: '数据看板',
            items: [
                { key: 'dataBoard', label: '访问数据看板' }
            ]
        }
    ];

    // 计算已选中的权限数量
    const calculateSelectedCount = useCallback(() => {
        const permissions = form.getFieldValue('permissions') || {};
        const count = Object.values(permissions).filter(Boolean).length;
        setSelectedCount(count);
    }, [form]);

    useEffect(() => {
        if (mode === 'edit' && roleId) {
            // TODO: 根据roleId获取角色信息
            // const roleData = await getRoleDetail(roleId);
            // 模拟数据
            const mockRoleData: RoleFormData = {
                roleName: '部门管理员',
                roleDescription: '部门管理员角色，具有部门级别的权限管理能力',
                permissions: {
                    contractReview: true,
                    riskIdentification: true,
                    contractModification: true,
                    contractExport: true,
                    contractSwitching: true,
                    contractComparison: false,
                    riskClassification: false,
                    contractExportAnnotation: false,
                    contractSwitchingStatus: false,
                    auditRecordSelf: true,
                    auditRecordDepartment: true,
                    auditRecordPlatform: false,
                    auditRecordOthers: false,
                    dataBoard: false
                }
            };
            form.setFieldsValue(mockRoleData);
            calculateSelectedCount();
        }
    }, [mode, roleId, form, calculateSelectedCount]);

    useEffect(() => {
        // 如果有section参数，滚动到对应位置
        if (section === 'permissions') {
            setTimeout(() => {
                const element = document.getElementById('permissions-section');
                if (element) {
                    element.scrollIntoView({ behavior: 'smooth', block: 'start' });
                }
            }, 0);
        }
    }, [section]);

    const handleSubmit = async (values: RoleFormData) => {
        try {
            setLoading(true);

            // TODO: 调用API
            // if (mode === 'add') {
            //     await addRole(values);
            // } else {
            //     await updateRole(roleId, values);
            // }

            message.success(mode === 'add' ? '新增角色成功' : '编辑角色成功');
            router.push('/setting/role');
        } catch (error) {
            message.error(mode === 'add' ? '新增角色失败' : '编辑角色失败');
        } finally {
            setLoading(false);
        }
    };

    const handleCancel = useCallback(() => {
        Modal.confirm({
            title: '确认退出编辑',
            icon: <InfoCircleOutlined />,
            content: '退出后，修改的内容将不会保存',
            okText: '退出',
            cancelText: '暂不退出',
            onOk() {
                router.back();
            },
            onCancel() {
                console.log('取消退出');
            }
        });
    }, [router]);

    // 监听权限变化
    const handlePermissionChange = () => {
        calculateSelectedCount();
    };

    return (
        <div className="flex flex-col bg-[#f1f1f1] h-[100%]">
            {/* 内容区域 */}
            <div className="bg-white flex-1 flex flex-col overflow-hidden p-[1rem] relative pb-0">
                <div className="flex-1 overflow-auto">
                    <div className="text-[1.25rem] text-black font-bold border-l-[0.31rem] border-[#2260F2] pl-[0.75rem] mb-6">
                        {title}
                    </div>
                    <Form
                        form={form}
                        layout="vertical"
                        onFinish={handleSubmit}
                        requiredMark={false}
                        onValuesChange={handlePermissionChange}
                    >
                        <div className="flex gap-12">
                            {/* 编辑角色信息 - 左侧 */}
                            <div className="w-1/2">
                                <div className='px-[2rem]'>
                                    <div className="flex items-center gap-3 mb-6 pb-3 border-b" style={{ borderColor: '#e5e5e5' }}>
                                        <div className="text-[1.25rem] text-black">
                                            角色信息
                                        </div>
                                    </div>
                                    <Form.Item
                                        label={
                                            <div className="flex items-baseline gap-2">
                                            <span className="text-base font-medium">
                                                角色名称
                                                <span className="text-red-500 ml-1">*</span>
                                            </span>
                                            </div>
                                        }
                                        name="roleName"
                                        rules={[
                                            { required: true, message: '请输入角色名称' },
                                            { max: 50, message: '角色名称不超过50个字符' }
                                        ]}
                                    >
                                        <Input
                                            placeholder="请输入角色名称"
                                            size="large"
                                            maxLength={50}
                                        />
                                    </Form.Item>

                                    <Form.Item
                                        label={
                                            <div className="flex items-baseline gap-2">
                                            <span className="text-base font-medium">
                                                角色说明
                                            </span>
                                            </div>
                                        }
                                        name="roleDescription"
                                    >
                                        <Input.TextArea
                                            placeholder="请输入角色说明，以帮助其他管理员了解该角色用途和主要事项"
                                            rows={12}
                                            maxLength={500}
                                            showCount
                                            style={{ fontFamily: 'inherit' }}
                                        />
                                    </Form.Item>
                                </div>
                            </div>

                            {/* 选择权限 - 右侧 */}
                            <div id="permissions-section" className="w-1/2">
                                <div className='px-[2rem]'>
                                    <div className="flex items-center gap-3 mb-6 pb-3 border-b" style={{ borderColor: '#e5e5e5' }}>
                                        <div className="text-[1.25rem] text-black">
                                            角色权限
                                        </div>
                                        <span className="text-sm text-gray-500">
                                        已选 <span className="text-[#2260F2]">{selectedCount}</span> 项权限
                                    </span>
                                    </div>

                                    {/* 循环渲染权限分组 */}
                                    {permissionSections.map((section, sectionIndex) => (
                                        <div key={sectionIndex} className="mb-8 bg-gray-50 p-4 rounded">
                                            <div className="text-base font-medium text-gray-800 pb-2 border-b" style={{ borderColor: '#e8e8e8' }}>
                                                {section.title}
                                            </div>
                                            <div className="pt-4">
                                                <div className="grid grid-cols-2 gap-4 md:grid-cols-3">
                                                    {section.items.map((item) => (
                                                        <Form.Item
                                                            key={item.key}
                                                            name={['permissions', item.key]}
                                                            valuePropName="checked"
                                                            noStyle
                                                        >
                                                            <Checkbox>{item.label}</Checkbox>
                                                        </Form.Item>
                                                    ))}
                                                </div>
                                            </div>
                                        </div>
                                    ))}
                                </div>
                            </div>
                        </div>
                    </Form>
                </div>

                {/* 按钮区域 - 固定在底部 */}
                <div className="flex justify-end py-2 bg-white">
                    <div className="flex items-center gap-3 px-12 py-2">
                        <Button onClick={handleCancel} className="px-8">
                            取消
                        </Button>
                        <Button
                            type="primary"
                            htmlType="submit"
                            loading={loading}
                            className="px-8 bg-blue-500"
                            onClick={() => form.submit()}
                        >
                            {mode === 'add' ? '创建角色' : '保存修改'}
                        </Button>
                    </div>
                </div>
            </div>
        </div>
    );
}

export default function RoleEditAndAddPageWithSuspense() {
    return (
        <Suspense fallback={null}>
            <RoleEditAndAddPage />
        </Suspense>
    );
}
