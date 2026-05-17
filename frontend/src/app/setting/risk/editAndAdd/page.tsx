'use client';

import React, { Suspense, useEffect, useMemo } from 'react';
import { Form, Input, Button, message, Select, Radio } from 'antd';
import { useRouter, useSearchParams } from 'next/navigation';
import { useRiskEditStore } from '@/store/riskEditStore';
import { ContractTypeListItem, getContractTypeList } from '@/lib/api/contractType';
import { createRiskPoint, updateRiskPoint } from '@/lib/api/risk';

interface RiskData {
    contractTypeId: number;
    applicableScope: 'individual' | 'department' | 'platform';
    department?: string[];
    riskContent: string;
    riskType: string;
    riskLevel: string;
    isEnabled: 'enabled' | 'disabled';
}

function RiskEditAndAddContent() {
    const [form] = Form.useForm<RiskData>();
    const [loading, setLoading] = React.useState(false);
    const [contractTypes, setContractTypes] = React.useState<ContractTypeListItem[]>([]);
    const router = useRouter();
    const searchParams = useSearchParams();
    const riskData = useRiskEditStore((state) => state.riskData);
    const clearRiskData = useRiskEditStore((state) => state.clearRiskData);

    const mode = (searchParams.get('mode') || 'add') as 'add' | 'edit';
    const selectedScope = Form.useWatch('applicableScope', form);

    const title = mode === 'add' ? '新建风险点' : '编辑风险点';

    useEffect(() => {
        const fetchContractTypes = async () => {
            try {
                const response = await getContractTypeList();
                if (response?.code === 200 && response?.data?.list) {
                    setContractTypes(response.data.list);
                }
            } catch {
                message.error('获取合同类型失败');
            }
        };
        fetchContractTypes();
    }, []);

    const queryContractType = searchParams.get('contractType');
    const queryContractTypeId = useMemo(() => {
        if (!queryContractType) return undefined;
        const found = contractTypes.find((item) => (item.contractTypeName || item.name) === queryContractType);
        return found ? Number(found.id) : undefined;
    }, [contractTypes, queryContractType]);

    useEffect(() => {
        if (mode === 'edit' && riskData.id) {
            form.setFieldsValue({
                contractTypeId: riskData.contractTypeId,
                applicableScope: riskData.applicableScope || 'platform',
                department: riskData.department || [],
                riskContent: riskData.riskContent || '',
                riskType: riskData.riskType || '',
                riskLevel: riskData.riskLevel || '中',
                isEnabled: riskData.isEnabled || 'enabled'
            });
        } else if (mode === 'add') {
            form.setFieldsValue({
                contractTypeId: queryContractTypeId,
                applicableScope: 'platform',
                department: [],
                riskType: '',
                riskLevel: '中',
                riskContent: '',
                isEnabled: 'enabled'
            });
        }
    }, [mode, queryContractTypeId, riskData, form]);

    const handleSubmit = async (values: RiskData) => {
        try {
            setLoading(true);

            const selectedType = contractTypes.find((item) => Number(item.id) === Number(values.contractTypeId));
            const submitData = {
                contractTypeId: Number(values.contractTypeId),
                contractType: selectedType?.contractTypeName || selectedType?.name,
                applicableScope: values.applicableScope,
                department: values.applicableScope === 'department' ? values.department || [] : [],
                riskContent: values.riskContent,
                riskType: values.riskType,
                riskLevel: values.riskLevel,
                isEnabled: values.isEnabled,
            };

            const response = mode === 'add'
                ? await createRiskPoint(submitData)
                : await updateRiskPoint(Number(riskData.id), submitData);

            if (response?.code === 200) {
                message.success(mode === 'add' ? '新增成功' : '编辑成功');
                clearRiskData();
                router.push('/setting/risk?refresh=' + Date.now());
            } else {
                message.error(response?.msg || '提交失败');
            }
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

    return (
        <div className="flex flex-col min-h-screen bg-white">
            <div className="w-[70%] bg-white p-6">
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
                            </div>
                        }
                        name="contractTypeId"
                        rules={[
                            { required: true, message: '请选择合同类型' }
                        ]}
                    >
                        <Select placeholder="请选择" size="large">
                            {contractTypes.map((item) => (
                                <Select.Option key={item.id} value={Number(item.id)}>
                                    {item.contractTypeName || item.name}
                                </Select.Option>
                            ))}
                        </Select>
                    </Form.Item>

                    <div className="grid grid-cols-2 gap-4">
                        <Form.Item
                            label={
                                <div className="flex items-baseline gap-2">
                                    <span className="text-base font-medium">
                                        风险类型
                                        <span className="text-red-500 ml-1">*</span>
                                    </span>
                                </div>
                            }
                            name="riskType"
                            rules={[
                                { required: true, message: '请输入风险类型' },
                                { max: 64, message: '风险类型不超过64个字符' }
                            ]}
                        >
                            <Input placeholder="如付款风险、验收风险、违约责任风险" size="large" />
                        </Form.Item>

                        <Form.Item
                            label={
                                <div className="flex items-baseline gap-2">
                                    <span className="text-base font-medium">
                                        风险等级
                                        <span className="text-red-500 ml-1">*</span>
                                    </span>
                                </div>
                            }
                            name="riskLevel"
                            rules={[
                                { required: true, message: '请选择风险等级' }
                            ]}
                        >
                            <Select size="large">
                                <Select.Option value="高">高</Select.Option>
                                <Select.Option value="中">中</Select.Option>
                                <Select.Option value="低">低</Select.Option>
                            </Select>
                        </Form.Item>
                    </div>

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

                    {selectedScope === 'department' && (
                        <Form.Item
                            label={
                                <div className="flex items-baseline gap-2">
                                    <span className="text-base font-medium">
                                        选择部门
                                        <span className="text-red-500 ml-1">*</span>
                                    </span>
                                </div>
                            }
                            name="department"
                            rules={[
                                { required: true, message: '请选择部门' }
                            ]}
                        >
                            <Select placeholder="请选择部门" mode="multiple">
                                <Select.Option value="销售部">销售部</Select.Option>
                                <Select.Option value="采购部">采购部</Select.Option>
                                <Select.Option value="法务部">法务部</Select.Option>
                                <Select.Option value="财务部">财务部</Select.Option>
                            </Select>
                        </Form.Item>
                    )}

                    <Form.Item
                        label={
                            <div className="flex items-baseline gap-2">
                                <span className="text-base font-medium">
                                    风险点内容
                                    <span className="text-red-500 ml-1">*</span>
                                </span>
                            </div>
                        }
                        name="riskContent"
                        rules={[
                            { required: true, message: '请输入风险内容' },
                            { max: 5000, message: '风险内容不超过5000个字符' }
                        ]}
                    >
                        <Input.TextArea
                            placeholder="请输入该类合同可能存在的风险点、触发条件和建议关注方式"
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
