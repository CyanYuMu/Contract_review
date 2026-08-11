'use client';

import React, {useEffect, useRef, useState, useMemo, useCallback} from 'react';
import {Card, Table, Segmented, Select, Statistic, Row, Col, Spin, Empty, message} from 'antd';
import * as echarts from 'echarts';
import {getUsageStats, getUsageTrend, listGatewayRoutes} from '@/lib/api/gateway';
import type {UsageStat, DailyUsageTrend, GatewayRoute} from '@/lib/Interface';

const FEATURE_LABELS: Record<string, string> = {
    review: '合同审阅',
    qa: '合同问答',
    comparison: '合同比对',
    chat: '聊天',
    default: '默认',
    embedding: '向量化',
};
const featureLabel = (f: string) => FEATURE_LABELS[f] || f;

/** 大模型成本看板：用量统计(用户+功能双维度) + 趋势 + 模型路由 */
export default function CostDashboard() {
    const [dimension, setDimension] = useState<'feature' | 'user'>('feature');
    const [days, setDays] = useState(30);
    const [stats, setStats] = useState<UsageStat[]>([]);
    const [trend, setTrend] = useState<DailyUsageTrend[]>([]);
    const [routes, setRoutes] = useState<GatewayRoute[]>([]);
    const [loading, setLoading] = useState(false);
    const chartRef = useRef<HTMLDivElement>(null);
    const chartInstance = useRef<echarts.ECharts | null>(null);

    const load = useCallback(async () => {
        setLoading(true);
        try {
            const [s, t, r] = await Promise.all([
                getUsageStats(dimension, days),
                getUsageTrend(days),
                listGatewayRoutes().catch(() => [] as GatewayRoute[]),
            ]);
            setStats(s || []);
            setTrend(t || []);
            setRoutes(r || []);
        } catch (e) {
            message.error(e instanceof Error ? e.message : '加载成本数据失败');
        } finally {
            setLoading(false);
        }
    }, [dimension, days]);

    useEffect(() => {
        load();
    }, [load]);

    const summary = useMemo(() => {
        const totalTokens = stats.reduce((a, b) => a + (b.total_tokens || 0), 0);
        const totalCost = stats.reduce((a, b) => a + (b.cost || 0), 0);
        const totalCalls = stats.reduce((a, b) => a + (b.call_count || 0), 0);
        const cacheHits = stats.reduce((a, b) => a + (b.cache_hit_count || 0), 0);
        return {totalTokens, totalCost, totalCalls, cacheHits};
    }, [stats]);

    // 趋势图渲染
    useEffect(() => {
        if (!chartRef.current) return;
        if (!chartInstance.current) {
            chartInstance.current = echarts.init(chartRef.current);
        }
        chartInstance.current.setOption({
            tooltip: {trigger: 'axis'},
            legend: {data: ['Token 用量', '调用次数', '花费(¥)'], top: 0},
            grid: {left: 50, right: 50, bottom: 30, top: 40, containLabel: true},
            xAxis: {type: 'category', data: trend.map((d) => d.date), axisLabel: {fontSize: 10}},
            yAxis: [
                {type: 'value', name: 'Token/次', position: 'left'},
                {type: 'value', name: '¥', position: 'right'},
            ],
            series: [
                {name: 'Token 用量', type: 'bar', data: trend.map((d) => d.total_tokens), itemStyle: {color: '#2260f2'}},
                {name: '调用次数', type: 'line', data: trend.map((d) => d.call_count), itemStyle: {color: '#52c41a'}},
                {name: '花费(¥)', type: 'line', yAxisIndex: 1, data: trend.map((d) => Number((d.cost || 0).toFixed(4))), itemStyle: {color: '#faad14'}},
            ],
        });
    }, [trend]);

    useEffect(() => {
        const onResize = () => chartInstance.current?.resize();
        window.addEventListener('resize', onResize);
        return () => {
            window.removeEventListener('resize', onResize);
            chartInstance.current?.dispose();
            chartInstance.current = null;
        };
    }, []);

    const statsColumns = [
        {
            title: dimension === 'user' ? '用户ID' : '功能模块',
            dataIndex: 'feature',
            render: (v: string) => (dimension === 'user' ? `用户 ${v}` : featureLabel(v)),
        },
        {title: '模型', dataIndex: 'model_name', render: (v: string) => v || '-'},
        {title: '调用次数', dataIndex: 'call_count'},
        {title: '输入 Token', dataIndex: 'prompt_tokens'},
        {title: '输出 Token', dataIndex: 'completion_tokens'},
        {title: '总 Token', dataIndex: 'total_tokens'},
        {title: '花费(¥)', dataIndex: 'cost', render: (v: number) => (v || 0).toFixed(4)},
        {title: '缓存命中', dataIndex: 'cache_hit_count'},
    ];

    return (
        <div className="p-2">
            <div className="flex items-center justify-between mb-4">
                <h2 className="text-lg font-semibold">大模型成本看板</h2>
                <div className="flex gap-3 items-center">
                    <Segmented
                        value={dimension}
                        onChange={(v) => setDimension(v as 'feature' | 'user')}
                        options={[
                            {label: '按功能模块', value: 'feature'},
                            {label: '按用户', value: 'user'},
                        ]}
                    />
                    <Select
                        value={days}
                        onChange={setDays}
                        options={[
                            {label: '近 7 天', value: 7},
                            {label: '近 30 天', value: 30},
                            {label: '近 90 天', value: 90},
                        ]}
                        style={{width: 120}}
                    />
                </div>
            </div>

            <Spin spinning={loading}>
                <Row gutter={16} className="mb-4">
                    <Col span={6}>
                        <Card><Statistic title="总 Token 用量" value={summary.totalTokens}/></Card>
                    </Col>
                    <Col span={6}>
                        <Card><Statistic title="总花费(¥)" value={summary.totalCost} precision={4}/></Card>
                    </Col>
                    <Col span={6}>
                        <Card><Statistic title="总调用次数" value={summary.totalCalls}/></Card>
                    </Col>
                    <Col span={6}>
                        <Card><Statistic title="缓存命中次数" value={summary.cacheHits}/></Card>
                    </Col>
                </Row>

                <Card title="用量趋势" className="mb-4" size="small">
                    <div ref={chartRef} style={{width: '100%', height: 300}}/>
                </Card>

                <Card title={dimension === 'user' ? '用户用量明细' : '功能模块用量明细'} size="small">
                    <Table
                        rowKey={(r: UsageStat) => `${r.feature}-${r.model_name}-${r.call_count}`}
                        columns={statsColumns}
                        dataSource={stats}
                        pagination={false}
                        size="small"
                        locale={{emptyText: <Empty description="暂无用量数据"/>}}
                    />
                </Card>

                <Card title="模型路由（功能 → 模型，在此切换模型无需改动应用代码）" className="mt-4" size="small">
                    <Table
                        rowKey="feature"
                        columns={[
                            {title: '功能', dataIndex: 'feature', render: featureLabel},
                            {title: '模型', dataIndex: 'model_name', render: (v: string) => v || '-'},
                            {title: '参数', dataIndex: 'params', render: (v: string) => v || '-'},
                            {title: '更新时间', dataIndex: 'updated_at'},
                        ]}
                        dataSource={routes}
                        pagination={false}
                        size="small"
                        locale={{emptyText: <Empty description="暂无路由配置"/>}}
                    />
                </Card>
            </Spin>
        </div>
    );
}
