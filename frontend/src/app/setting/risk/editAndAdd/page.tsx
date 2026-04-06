
'use client';

import React, { Suspense, useEffect } from 'react';
import { Form, Input, Button, message, Space, Select, Radio } from 'antd';
import { useRouter, useSearchParams } from 'next/navigation';
import type { FormInstance } from 'antd';
import { useRiskEditStore } from '@/store/riskEditStore';

interface RiskData {
    id?: string;
    contractType: string;
    applicableScope: 'individual' | 'department' | 'platform';
    department?: string[];
    riskContent: string;
    isEnabled: 'enabled' | 'disabled';
}

function RiskEditAndAddContent() {
    const [form] = Form.useForm<RiskData>();
    const [loading, setLoading] = React.useState(false);
    const router = useRouter();
    const searchParams = useSearchParams();
    const riskData = useRiskEditStore((state) => state.riskData);
    const clearRiskData = useRiskEditStore((state) => state.clearRiskData);
    
    const mode = (searchParams.get('mode') || 'add') as 'add' | 'edit';
    const [selectedDepartments, setSelectedDepartments] = React.useState<string[]>([]);

    const title = mode === 'add' ? '新建风险点' : '编辑风险点';

    useEffect(() => {
        if (mode === 'edit' && riskData.riskId) {
            // 填充表单数据
            form.setFieldsValue({
                contractType: riskData.contractType,
                applicableScope: riskData.applicableScope || 'individual',
                riskContent: riskData.riskContent,
                isEnabled: riskData.isEnabled || 'enabled'
            });
            setSelectedDepartments(riskData.department || []);
        } else {
            form.resetFields();
            setSelectedDepartments([]);
        }
    }, [mode, riskData, form]);

    const handleSubmit = async (values: RiskData) => {
        try {
            setLoading(true);

            const submitData: RiskData = {
                ...values,
                department: selectedDepartments,
                ...(mode === 'edit' && riskData.id && { id: riskData.id })
            };

            // 调用API
            // if (mode === 'add') {
            //     await addRisk(submitData);
            // } else {
            //     await editRisk(submitData);
            // }

            message.success(mode === 'add' ? '新增成功' : '编辑成功');
            
            // 清空状态管理器数据
            clearRiskData();
            
            // 返回上一页并刷新
            router.push('/setting/risk?refresh=' + Date.now());
        } catch (error) {
            console.error('提交失败:', error);
            message.error('提交失败，请稍后重试');
        } finally {
            setLoading(false);
        }
    };

    const handleCancel = () => {
        clearRiskData();
        router.back();
    };

    const handleAddDepartment = (checkedValues: string[]) => {
        setSelectedDepartments(checkedValues);
    };

    return (
        <div className="flex flex-col min-h-screen bg-white">
            <div className="w-[70%] bg-white  p-6">
                <div className='text-[1.25rem] text-black font-bold border-l-[0.31rem] border-[#2260F2] pl-[0.75rem] mb-[2rem]'>
                    {title}
                </div>
                <Form
                    form={form}
                    layout="vertical"
                    onFinish={handleSubmit}
                    requiredMark={false}
                    className='!pl-[2rem]'
                >
                    <Form.Item
                        label={
                            <div className="flex items-baseline gap-2">
                                <span className="text-base font-medium">
                                    合同类型
                                    <span className="text-red-500 ml-1">*</span>
                                </span>
                                <span className="text-xs text-gray-400">选择风险点适用的合同类型</span>
                            </div>
                        }
                        name="contractType"
                        rules={[
                            { required: true, message: '请选择合同类型' }
                        ]}
                    >
                        <Select
                            placeholder="请选择"
                            size="large"
                        >
                            <Select.Option value="货物类合同">货物类合同</Select.Option>
                            <Select.Option value="服务类合同">服务类合同</Select.Option>
                            <Select.Option value="工程类合同">工程类合同</Select.Option>
                        </Select>
                    </Form.Item>

                    <Form.Item
                        label={
                            <div className="flex items-baseline gap-2">
                                <span className="text-base font-medium">
                                    适用范围
                                    <span className="text-red-500 ml-1">*</span>
                                </span>
                            </div>
                        }
                        name="applicableScope"
                        rules={[
                            { required: true, message: '请选择适用范围' }
                        ]}
                    >
                        <Radio.Group className='!pl-[1rem]'>
                            <Radio value="individual">个人</Radio>
                            <Radio value="department">部门</Radio>
                            <Radio value="platform">平台</Radio>
                        </Radio.Group>
                    </Form.Item>

                    <Form.Item
                        label={
                            <div className="flex items-baseline gap-2">
                                <span className="text-base font-medium">
                                    选择部门
                                    <span className="text-red-500 ml-1">*</span>
                                </span>
                                <span className="text-xs text-gray-400">选择部门后，仅限该部门员工使用此风险点</span>
                            </div>
                        }
                        name="department"
                        rules={[
                            { required: true, message: '请选择部门' }
                        ]}
                    >
                        <Select
                            placeholder="请选择部门"
                            mode="multiple"
                            value={selectedDepartments}
                            onChange={handleAddDepartment}
                        >
                            <Select.Option value="销售部">销售部</Select.Option>
                            <Select.Option value="采购部">采购部</Select.Option>
                            <Select.Option value="法务部">法务部</Select.Option>
                            <Select.Option value="财务部">财务部</Select.Option>
                        </Select>
                    </Form.Item>

                    <Form.Item
                        label={
                            <div className="flex items-baseline gap-2">
                                <span className="text-base font-medium">
                                    风险点内容
                                    <span className="text-red-500 ml-1">*</span>
                                </span>
                                <span className="text-xs text-gray-400">详细描述风险点的具体内容</span>
                            </div>
                        }
                        name="riskContent"
                        rules={[
                            { required: true, message: '请输入风险内容' },
                            { max: 5000, message: '风险内容不超过5000个字符' }
                        ]}
                    >
                        <Input.TextArea
                            placeholder="请输入风险点内容"
                            rows={12}
                            className="resize-none"
                            style={{ fontFamily: 'inherit' }}
                        />
                    </Form.Item>

                    <Form.Item
                        label={
                            <div className="flex items-baseline gap-2">
                                <span className="text-base font-medium">
                                    是否启用
                                    <span className="text-red-500 ml-1">*</span>
                                </span>
                            </div>
                        }
                        name="isEnabled"
                        rules={[
                            { required: true, message: '请选择状态' }
                        ]}
                    >
                        <Radio.Group className='!pl-[1rem]'>
                            <Radio value="enabled">启用</Radio>
                            <Radio value="disabled">停用</Radio>
                        </Radio.Group>
                    </Form.Item>

                    <Form.Item>
                        <div className="flex justify-start gap-3 pt-6 ">
                            <Button
                                onClick={handleCancel}
                                className="px-8"
                            >
                                {mode === 'add' ? '取消新建' : '取消修改'}
                            </Button>
                            <Button
                                type="primary"
                                htmlType="submit"
                                loading={loading}
                                className="px-8"
                            >
                                {mode === 'add' ? '新建风险点' : '保存修改'}
                            </Button>
                        </div>
                    </Form.Item>
                </Form>
            </div>
        </div>
    );
}

export default function RiskEditAndAdd() {
    return (
        <Suspense fallback={null}>
            <RiskEditAndAddContent />
        </Suspense>
    );
}
