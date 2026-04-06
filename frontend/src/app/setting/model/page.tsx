'use client';

import React, {useEffect, useRef, useState} from 'react';
import {Button, Input, message} from 'antd';
import {getDefaultModel, updateModel, createModel} from "@/lib/api/model";

export default function ModelForm() {
    const [loading, setLoading] = useState(false);
    const [saving, setSaving] = useState(false);
    const [modelId, setModelId] = useState<number | string | null>(null);
    const [urlValue, setUrlValue] = useState<string>('');
    const [apiValue, setApiValue] = useState<string>('');
    const hasFetched = useRef(false);

    useEffect(() => {
        // 防止重复请求
        if (hasFetched.current) return;
        hasFetched.current = true;

        async function fetchDefaultModel() {
            setLoading(true);
            try {
                const data = await getDefaultModel();
                if (data) {
                    setModelId(data.id || null);
                    setUrlValue(data.api_url || '');
                    setApiValue(data.api_key || '');
                }
            } catch (error) {
                console.error('获取模型配置失败:', error);
            } finally {
                setLoading(false);
            }
        }

        fetchDefaultModel();
    }, [])

    const handleSave = async () => {
        setSaving(true);
        try {
            if (modelId) {
                // 有模型ID，更新模型
                await updateModel(modelId, {
                    api_url: urlValue,
                    api_key: apiValue
                });
                message.success('保存成功');
            } else {
                // 没有模型ID，创建模型
                const result = await createModel({
                    api_url: urlValue,
                    api_key: apiValue
                });
                if (result?.id) {
                    setModelId(result.id);
                }
                message.success('创建成功');
            }
        } catch (error) {
            console.error('保存失败:', error);
        } finally {
            setSaving(false);
        }
    };

    return (
        <div className="p-6">
            <div className="max-w-4xl">
                <div className='text-[1.25rem] text-black font-bold border-l-[0.31rem] border-[#2260F2] pl-[0.75rem] mb-[2rem]'>
                    大模型配置
                </div>
                
                <div className='ml-[1.5rem]'>
                    <div>
                        <div className='text-[1.13rem] mb-[0.88rem] font-medium text-black'>接入大模型</div>
                        <Input
                            placeholder='请输入URL'
                            className='!w-[46.75rem] !mb-[0.75rem]'
                            value={urlValue}
                            onChange={(e) => setUrlValue(e.target.value)}
                        />
                        <Input
                            placeholder='请输入KEY'
                            className='!w-[46.75rem]'
                            value={apiValue}
                            onChange={(e) => setApiValue(e.target.value)}
                        />
                    </div>
                    
                    <div className='mt-[2rem]'>
                            <Button
                                className='!text-[1rem] !font-medium !py-[1.2rem] !px-[1.4rem]'
                                type='primary'
                                loading={saving}
                                onClick={handleSave}
                            >
                                保存
                            </Button>
                    </div>
                </div>
            </div>
        </div>
    );
}