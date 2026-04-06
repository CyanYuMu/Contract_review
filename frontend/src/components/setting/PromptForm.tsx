import React, {useEffect, useState} from 'react';
import {Button, Form, Input} from 'antd';
import {getOrganizationPrompt} from "@/lib/api/getOrganizationPrompt";

export default function PromptForm() {
    const {TextArea} = Input;
    const [changeForm, setChangeForm] = React.useState<boolean>(false);
    const [servicePrompt, setServicePrompt] = useState('');
    const [infraPrompt, setInfraPrompt] = useState('');
    const [goodsPrompt, setGoodsPrompt] = useState('');
    const [loading, setLoading] = useState(false);
    useEffect(() => {
        async function getAllPrompts() {
            try {
                setLoading(true);
                const [
                    goodsText,
                    serviceText,
                    infraText,
                ] = await Promise.all([
                    getOrganizationPrompt({contract_type_id: 2, organization_id: 3}), // 货物
                    getOrganizationPrompt({contract_type_id: 1, organization_id: 1}), // 服务
                    getOrganizationPrompt({contract_type_id: 3, organization_id: 4}), // 基建
                ]);

                setGoodsPrompt(goodsText || '');
                setServicePrompt(serviceText || '');
                setInfraPrompt(infraText || '');
            } catch (err) {
                console.error(err);
            } finally {
                setLoading(false);
            }
        }

        getAllPrompts();
    }, []);


    return (
        <div>
            <div
                className='text-[1.25rem] text-black font-bold border-l-[0.31rem] border-[#2260F2] pl-[0.75rem] mb-[2rem]'>提示词配置
            </div>
            <Form className='!ml-[3.5rem]'>
                <Form.Item>
                    <div className='text-[1.13rem] mb-[0.88rem] font-medium'>服务类合同</div>
                    <TextArea
                        className='!w-[46.75rem] !h-[6.25rem]'
                        maxLength={8000}
                        showCount
                        value={servicePrompt}
                    />
                </Form.Item>
                <Form.Item>
                    <div className='text-[1.13rem] mb-[0.88rem] font-medium'>基建类合同</div>
                    <TextArea
                        className='!w-[46.75rem] !h-[6.25rem]'
                        maxLength={8000}
                        showCount
                        value={infraPrompt}
                    />
                </Form.Item>
                <Form.Item>
                    <div className='text-[1.13rem] mb-[0.88rem] font-medium'>货物类合同</div>
                    <TextArea
                        className='!w-[46.75rem] !h-[6.25rem]'
                        maxLength={8000}
                        showCount
                        value={goodsPrompt}
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
