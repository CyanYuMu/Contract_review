'use client';
import React, { useState } from 'react';
import { Button, Form, Input, Modal } from 'antd';
import toast from 'react-hot-toast';
import { LockOutlined, UserOutlined } from '@ant-design/icons';
import { register } from '@/lib/api/register';
import Image from 'next/image';

interface RegisterModalProps {
    visible: boolean;
    onCancel: () => void;
    onSuccess: () => void;
    onSwitchToLogin: () => void;
}

export default function RegisterModal({ visible, onCancel, onSuccess, onSwitchToLogin }: RegisterModalProps) {
    const [loading, setLoading] = useState(false);

    const handleRegister = async (values: { username: string, password: string, confirmPassword: string }) => {
        try {
            setLoading(true);
            await register({
                account: values.username,
                username: values.username,
                password: values.password
            });
            toast.success('注册成功，请登录');
            onSuccess();
        } catch (err) {
            if (err instanceof Error) {
                toast.error(err.message || '注册失败，请稍后重试');
            } else {
                toast.error('注册失败，请稍后重试');
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
                        <h1 className="text-2xl font-bold text-gray-800">注册账号</h1>
                    </div>

                    <Form layout="vertical" onFinish={handleRegister} requiredMark={false}>
                        <Form.Item
                            name="username"
                            label="用户名"
                            rules={[
                                { required: true, message: '请输入用户名' },
                            ]}
                        >
                            <Input
                                placeholder="请输入用户名"
                                prefix={<UserOutlined className="text-gray-400" />}
                                autoComplete="username"
                                size="large"
                            />
                        </Form.Item>

                        <Form.Item
                            name="password"
                            label="密码"
                            rules={[
                                { required: true, message: '请输入密码' },
                                { min: 6, message: '密码至少6位' },
                            ]}
                        >
                            <Input.Password
                                placeholder="请输入密码"
                                prefix={<LockOutlined className="text-gray-400" />}
                                autoComplete="new-password"
                                size="large"
                            />
                        </Form.Item>

                        <Form.Item
                            name="confirmPassword"
                            label="确认密码"
                            dependencies={['password']}
                            rules={[
                                { required: true, message: '请确认密码' },
                                ({ getFieldValue }) => ({
                                    validator(_, value) {
                                        if (!value || getFieldValue('password') === value) {
                                            return Promise.resolve();
                                        }
                                        return Promise.reject(new Error('两次输入的密码不一致'));
                                    },
                                }),
                            ]}
                        >
                            <Input.Password
                                placeholder="请再次输入密码"
                                prefix={<LockOutlined className="text-gray-400" />}
                                autoComplete="new-password"
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
                                {loading ? '注册中…' : '注册'}
                            </Button>
                        </Form.Item>

                        <div className="text-center mt-4">
                            <span className="text-gray-600">已有账号？</span>
                            <button
                                className="text-blue-600 hover:text-blue-700 font-medium ml-1"
                                onClick={onSwitchToLogin}
                            >
                                点击登录
                            </button>
                        </div>
                    </Form>
                </div>
            </div>
        </Modal>
    );
}
