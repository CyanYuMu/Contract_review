'use client';

import '@ant-design/v5-patch-for-react-19';
import React, { useState, useEffect, Suspense } from 'react';
import { Button, Form, Input, Select, Radio, message } from 'antd';
import { useRouter, useSearchParams } from 'next/navigation';

const { TextArea } = Input;

function PermissionEditContent() {
    const [form] = Form.useForm();
    const [loading, setLoading] = useState(false);
    const router = useRouter();
    const searchParams = useSearchParams();
    const mode = searchParams.get('mode') || 'add';
    const permissionId = searchParams.get('id');

    const isEdit = mode === 'edit';

    // 所属模块选项
    const moduleOptions = [
        { label: '角色配置', value: '角色配置' },
        { label: '模块1', value: '模块1' },
        { label: '模块2', value: '模块2' },
        { label: '模块3', value: '模块3' },
        { label: '模块4', value: '模块4' },
        { label: '模块5', value: '模块5' },
    ];

    useEffect(() => {
        if (isEdit && permissionId) {
            // TODO: 调用接口获取权限详情
            // 模拟数据
            form.setFieldsValue({
                permissionId: permissionId,
                permissionName: '权限名称1',
                permissionDesc: '这是权限1的权限描述，可以帮助其他管理员了解到该权限的详细内容。',
                module: '角色配置',
                status: 'enabled',
            });
        } else {
            // 新增时设置默认值
            form.setFieldsValue({
                permissionId: '123456',
                status: 'enabled',
                module: '角色配置',
            });
        }
    }, [isEdit, permissionId, form]);

    const handleSubmit = async (values: any) => {
        setLoading(true);
        try {
            // TODO: 调用保存接口
            console.log('提交数据:', values);
            message.success(isEdit ? '权限修改成功' : '权限新增成功');
            router.push('/setting/permission');
        } catch (error) {
            message.error(isEdit ? '权限修改失败' : '权限新增失败');
        } finally {
            setLoading(false);
        }
    };

    const handleCancel = () => {
        router.push('/setting/permission');
    };

    return (
        <div className="flex flex-col h-full bg-white p-6">
            {/* 标题 */}
            <div className="flex items-center mb-6">
                <div className="w-1 h-5 bg-[#2260F2] mr-3"></div>
                <h2 className="m-0 text-base font-medium text-black">
                    {isEdit ? '编辑权限' : '新增权限'}
                </h2>
            </div>

            {/* 表单内容 */}
            <div className="flex-1 overflow-auto">
                {/* 权限信息小标题 */}
                <div className="text-sm text-gray-800 font-medium mb-4">权限信息</div>

                <Form
                    form={form}
                    layout="vertical"
                    onFinish={handleSubmit}
                    className="max-w-[450px]"
                >
                    <Form.Item
                        label={
                            <span>
                                权限ID<span className="text-red-500">*</span>
                                <span className="text-gray-400 text-xs ml-2">系统自动生成，用于标识不同权限</span>
                            </span>
                        }
                        name="permissionId"
                        required={false}
                    >
                        <Input 
                            disabled 
                            placeholder="123456"
                            className="!w-[200px] !bg-white"
                        />
                    </Form.Item>

                    <Form.Item
                        label={<span>权限名称<span className="text-red-500">*</span></span>}
                        name="permissionName"
                        rules={[{ required: true, message: '请输入权限名称' }]}
                        required={false}
                    >
                        <Input 
                            placeholder="请输入权限名称" 
                            maxLength={50} 
                            className="!w-[200px]"
                        />
                    </Form.Item>

                    <Form.Item
                        label={<span>启用状态<span className="text-red-500">*</span></span>}
                        name="status"
                        rules={[{ required: true, message: '请选择启用状态' }]}
                        required={false}
                    >
                        <Radio.Group>
                            <Radio value="enabled">启用</Radio>
                            <Radio value="disabled">停用</Radio>
                        </Radio.Group>
                    </Form.Item>

                    <Form.Item
                        label={
                            <span>
                                所属模块<span className="text-red-500">*</span>
                                <span className="text-gray-400 text-xs ml-2">为权限{isEdit ? '划入' : '备注'}模块，方便管理</span>
                            </span>
                        }
                        name="module"
                        rules={[{ required: true, message: '请选择所属模块' }]}
                        required={false}
                    >
                        <Input 
                            disabled
                            className="!w-[200px] !bg-white"
                        />
                    </Form.Item>

                    <Form.Item
                        label={<span>权限描述<span className="text-red-500">*</span></span>}
                        name="permissionDesc"
                        rules={[{ required: true, message: '请输入权限描述' }]}
                        required={false}
                    >
                        <TextArea
                            placeholder="请输入权限描述"
                            rows={5}
                            maxLength={500}
                            className="!w-[450px]"
                        />
                    </Form.Item>
                </Form>
            </div>

            {/* 底部按钮 */}
            <div className="pt-6 flex gap-3">
                <Button onClick={handleCancel}>
                    {isEdit ? '退出编辑' : '取消新增'}
                </Button>
                <Button type="primary" loading={loading} onClick={() => form.submit()}>
                    {isEdit ? '保存修改' : '确认新增'}
                </Button>
            </div>
        </div>
    );
}

export default function PermissionEditPage() {
    return (
        <Suspense fallback={<div className="flex items-center justify-center h-full">加载中...</div>}>
            <PermissionEditContent />
        </Suspense>
    );
}
