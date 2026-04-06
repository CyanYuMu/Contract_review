
'use client';

import React, { Suspense, useEffect } from 'react';
import { Button, Input, Form, message } from 'antd';
import { useRouter, useSearchParams } from 'next/navigation';
import { useContractTypeEditStore } from '@/store/contractTypeEditStore';
import { createContractType, updateContractType } from '@/lib/api/contractType';

interface ContractTypeForm {
    contractTypeName: string;
    templateContent: string;
}

function ContractTypeEditContent() {
    const [form] = Form.useForm<ContractTypeForm>();
    const router = useRouter();
    const searchParams = useSearchParams();
    const mode = searchParams.get('mode') as 'add' | 'edit' | null;

    const { contractTypeData, clearContractTypeData } = useContractTypeEditStore();
    const [submitting, setSubmitting] = React.useState(false);

    useEffect(() => {
        // 如果是编辑模式，填充表单数据
        if (mode === 'edit' && contractTypeData?.contractTypeName) {
            form.setFieldsValue({
                contractTypeName: contractTypeData.contractTypeName || '',
                templateContent: contractTypeData.templateContent || ''
            });
        } else if (mode === 'add') {
            form.resetFields();
        }
    }, [mode, contractTypeData, form]);

    // 提交表单
    const handleSubmit = async (values: ContractTypeForm) => {
        setSubmitting(true);
        try {
            if (mode === 'add') {
                const response = await createContractType({
                    contractTypeName: values.contractTypeName,
                    templateContent: values.templateContent
                });
                if (response?.code === 200) {
                    message.success('添加成功');
                    clearContractTypeData();
                    router.push('/setting/contractType');
                } else {
                    message.error(response?.msg || '添加失败');
                }
            } else if (mode === 'edit') {
                const response = await updateContractType(Number(contractTypeData?.id), {
                    contractTypeName: values.contractTypeName,
                    templateContent: values.templateContent
                });
                if (response?.code === 200) {
                    message.success('编辑成功');
                    clearContractTypeData();
                    router.push('/setting/contractType');
                } else {
                    message.error(response?.msg || '编辑失败');
                }
            }
        } catch (error) {
            message.error('操作失败');
        } finally {
            setSubmitting(false);
        }
    };

    // 返回列表
    const handleCancel = () => {
        clearContractTypeData();
        router.push('/setting/contractType');
    };

    return (
        <div className="flex flex-col bg-white">
            <div className="bg-white p-6">
                <div className='text-[1.25rem] text-black font-bold border-l-[0.31rem] border-[#2260F2] pl-[0.75rem] mb-[2rem]'>
                    {mode === 'add' ? '新增合同类型' : '编辑合同类型'}
                </div>
                <Form
                    form={form}
                    layout="vertical"
                    onFinish={handleSubmit}
                    autoComplete="off"
                    requiredMark={false}
                    className='!px-[2rem]'
                >
                    <Form.Item
                        label={
                            <div className="flex items-baseline gap-2">
                                <span className="text-base font-medium">
                                    合同类型名称
                                    <span className="text-red-500 ml-1">*</span>
                                </span>
                            </div>
                        }
                        name="contractTypeName"
                        rules={[
                            { required: true, message: '请输入合同类型名称' },
                            { max: 50, message: '合同类型名称不超过50个字符' }
                        ]}
                    >
                        <Input
                            placeholder="请输入合同类型名称"
                            maxLength={50}
                            size="large"
                        />
                    </Form.Item>

                    <Form.Item
                        label={
                            <div className="flex items-baseline gap-2">
                                <span className="text-base font-medium">
                                    提示词模板
                                    <span className="text-red-500 ml-1">*</span>
                                </span>
                                <span className="text-xs text-gray-400">输入AI审查合同时使用的提示词模板</span>
                            </div>
                        }
                        name="templateContent"
                        rules={[
                            { required: true, message: '请输入提示词模板' },
                            { max: 5000, message: '提示词模板不超过5000个字符' }
                        ]}
                    >
                        <Input.TextArea
                            placeholder="请输入提示词模板"
                            rows={12}
                            maxLength={5000}
                            showCount
                            style={{ fontFamily: 'inherit' }}
                        />
                    </Form.Item>

                    <Form.Item>
                        <div className="flex justify-end gap-3 pt-6">
                            <Button
                                onClick={handleCancel}
                                className="px-8"
                            >
                                取消
                            </Button>
                            <Button
                                type="primary"
                                htmlType="submit"
                                loading={submitting}
                                className="px-8 bg-blue-600 hover:bg-blue-700"
                            >
                                {mode === 'add' ? '新增合同类型' : '保存修改'}
                            </Button>
                        </div>
                    </Form.Item>
                </Form>
            </div>
        </div>
    );
}

export default function ContractTypeEditPage() {
    return (
        <Suspense fallback={null}>
            <ContractTypeEditContent />
        </Suspense>
    );
}
