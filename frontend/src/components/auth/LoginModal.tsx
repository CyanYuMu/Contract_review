'use client';
import React, {useState} from 'react';
import {Button, Form, Input, Modal} from 'antd';
import toast from 'react-hot-toast';
import {LockOutlined, UserOutlined} from '@ant-design/icons';
import {login} from '@/lib/api/login';
import {saveTokenInfo} from '@/utils/client'; // 添加这个导入
import Image from 'next/image';

interface LoginModalProps {
    visible: boolean;
    onCancel: () => void;
    onSuccess: (token: string) => void;
    onSwitchToRegister: () => void;
}

export default function LoginModal({visible, onCancel, onSuccess, onSwitchToRegister}: LoginModalProps) {
    const [loading, setLoading] = useState(false);

    const handleLogin = async (values: { account: string, password: string }) => {
        try {
            setLoading(true);
            const response = await login(values);

            type TokenData = {
                access_token?: string;
                refresh_token?: string;
                token_type?: string;
                [key: string]: unknown;
            };

            // 提取响应数据（兼容不同的响应格式）
            let tokenData: TokenData | undefined;
            const respData = response?.data as TokenData | { data?: TokenData } | undefined;
            if (respData && typeof respData === "object" && "data" in respData && typeof respData.data === "object") {
                tokenData = respData.data as TokenData;  // 格式: { data: { data: { access_token, ... } } }
            } else if (respData && typeof respData === "object") {
                tokenData = respData as TokenData;  // 格式: { data: { access_token, ... } }
            }

            if (tokenData && tokenData.access_token) {
                // 使用统一的 saveTokenInfo 函数保存 token
                saveTokenInfo({
                    access_token: tokenData.access_token,
                    refresh_token: tokenData.refresh_token,
                    token_type: tokenData.token_type || 'Bearer'
                });

                // 验证 token 是否成功保存
                const savedToken = localStorage.getItem('access_token');
                if (savedToken !== tokenData.access_token) {
                    throw new Error('Token 保存失败，请检查浏览器设置');
                }

                console.log('✅ Token 已保存到 localStorage');
                console.log('验证 - access_token:', savedToken?.substring(0, 20) + '...');

                toast.success('登录成功');
                onSuccess(tokenData.access_token);
            } else {
                throw new Error('登录响应格式错误，未找到 access_token');
            }
        } catch (err) {
            if (err instanceof Error) {
                toast.error(err.message || '登录失败，请稍后重试');
            } else {
                toast.error('登录失败，请稍后重试');
            }
        } finally {
            setLoading(false);
        }
    };

    return (
        <Modal
            title={null}
            open={visible}
            onCancel={onCancel}
            footer={null}
            width={500}
            centered
            maskClosable={true}
            closable={true}
        >
            <div className="flex justify-center">
                <div className="w-full max-w-md p-8">
                    <div className="flex items-center mb-8">
                        <Image
                            src="/LogoIcon.png"
                            alt="Logo"
                            width={40}
                            height={40}
                            className="w-10 h-10 mr-3"
                            priority
                        />
                        <h1 className="text-2xl font-bold text-gray-800">AI合同审查</h1>
                    </div>

                    <Form layout="vertical" onFinish={handleLogin} requiredMark={false}>
                        <Form.Item
                            name="account"
                            label="用户名"
                            rules={[
                                {required: true, message: '请输入用户名'},
                            ]}
                        >
                            <Input
                                placeholder="请输入用户名"
                                prefix={<UserOutlined className="text-gray-400"/>}
                                autoComplete="username"
                                size="large"
                            />
                        </Form.Item>

                        <Form.Item
                            name="password"
                            label="密码"
                            rules={[
                                {required: true, message: '请输入密码'},
                            ]}
                        >
                            <Input.Password
                                placeholder="请输入密码"
                                prefix={<LockOutlined className="text-gray-400"/>}
                                autoComplete="current-password"
                                size="large"
                            />
                        </Form.Item>

                        <Form.Item>
                            <Button
                                type="primary"
                                htmlType="submit"
                                block
                                loading={loading}
                                size="large"
                                className="bg-purple-600 hover:bg-purple-700 border-purple-600 hover:border-purple-700 h-12 text-lg font-medium"
                            >
                                {loading ? '提交中…' : '登录'}
                            </Button>
                        </Form.Item>

                        <div className="text-center mt-4">
                            <span className="text-gray-600">没有账号？</span>
                            <button
                                className="text-blue-600 hover:text-blue-700 font-medium ml-1"
                                onClick={onSwitchToRegister}
                            >
                                点击注册账号
                            </button>
                        </div>
                    </Form>
                </div>
            </div>
        </Modal>
    );
}
