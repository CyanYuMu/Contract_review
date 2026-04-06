import React, {useEffect, useRef, useState} from 'react';
import {Button, Form, Input, Select, SelectProps} from 'antd';
import {getAllModels} from "@/lib/api/getAllModels";

export default function ModelForm() {
    const [options, setOptions] = useState<SelectProps['options']>([]);
    const [loading, setLoading] = useState(false);
    const [changeForm, setChangeForm] = React.useState<boolean>(false);
    const [modelId, setModelId] = useState<number>(1);
    const hasFetched = useRef(false);

    useEffect(() => {
        // 防止重复请求
        if (hasFetched.current) return;
        hasFetched.current = true;

        async function getModels() {
            setLoading(true);
            try {
                const models = await getAllModels();
                const opts = models.map((m: { id: number; model_name: string }) => ({
                    label: m.model_name,
                    value: m.id
                }))
                setOptions(opts);
            } finally {
                setLoading(false);
            }
        }

        getModels();
    }, [])
    return (
        <div>
            <div
                className='text-[1.25rem] text-black font-bold border-l-[0.31rem] border-[#2260F2] pl-[0.75rem] mb-[2rem]'>大模型配置
            </div>
            <Form className='!ml-[3.5rem]'>
                {/* <Form.Item>
                    <div className='text-[1.13rem] mb-[0.88rem] font-medium'>选择大模型</div>
                    <Select
                        className='!w-[14.6rem]'
                        options={options}
                        onChange={(v) => setModelId(v)}
                        value={modelId}
                        loading={loading}
                    />
                </Form.Item> */}
                <Form.Item>
                    <div className='text-[1.13rem] mb-[0.88rem] font-medium'>接入大模型</div>
                    <Input
                        placeholder='请输入URL'
                        className='!w-[46.75rem] !mb-[0.75rem]'
                    />
                    <Input
                        placeholder='请输入API'
                        className='!w-[46.75rem]'
                    />
                </Form.Item>
                <div className='mt-[1rem]'>
                    {changeForm === false ? (
                        <Button
                            className='!text-[1rem] !font-medium'
                            type='primary'
                            onClick={() => {
                                setChangeForm(true)
                            }}
                        >修改</Button>
                    ) : (
                        <>
                            <Button
                                className='!text-[1rem] !font-medium'
                                onClick={() => setChangeForm(false)}
                            >取消</Button>
                            <Button
                                type='primary'
                                className='ml-[1rem] !text-[1rem] !font-medium'
                                onClick={() => setChangeForm(false)}
                            >保存</Button>
                        </>
                    )}
                </div>
            </Form>
        </div>
    );
};
