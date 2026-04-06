'use client';
import React, { useEffect, useRef } from 'react';
import * as echarts from 'echarts';

const DepartmentRank: React.FC = () => {
    const chartRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        if (!chartRef.current) return;
        const chart = echarts.init(chartRef.current);

        const data = [
            { name: '条款引用错误', value: 100, rank: 1 },
            { name: '合同主体错误', value: 95, rank: 2 },
            { name: '金额计算错误', value: 90, rank: 3 },
            { name: '签字盖章缺失', value: 85, rank: 4 },
            { name: '货物数目错误', value: 80, rank: 5 },
        ];

        const maxValue = Math.max(...data.map(item => item.value));
        const rankIcons = [' 🥇', ' 🥈', ' 🥉', '  4', '  5'];
        // const iconColors = ['#FFD700', '#C0C0C0', '#CD7F32', '#000000', '#000000'];

        const option = {
            title: { show: false },
            grid: {
                left: '25px',
                right: '5px',
                top: '30px',
                bottom: '2px',
                containLabel: true,
            },
            tooltip: {
                trigger: 'axis',
                axisPointer: { type: 'shadow' },
                formatter: (params: any) => {
                    const item = data[params[0].dataIndex];
                    return `${item.name}: ${item.value.toLocaleString()}`;
                }
            },
            xAxis: {
                type: 'value',
                axisLine: { show: false },
                axisTick: { show: false },
                splitLine: { show: false },
                axisLabel: { show: false }
            },
            yAxis: [{
                type: 'category',
                data: data.map(item => item.name),
                inverse: true,
                axisLine: { show: false },
                axisTick: { show: false },
                axisLabel: { show: false }
            }, {
                type: 'category',
                data: data.map(() => maxValue),
                inverse: true,
                axisLine: { show: false },
                axisTick: { show: false },
                axisLabel: { show: false }
            }],
            series: [
                {
                    type: 'bar',
                    data: data.map(() => maxValue),
                    barWidth: 20,
                    barGap: '-100%',
                    itemStyle: {
                        color: 'rgba(242, 248, 255, 1)',
                        borderRadius: 2,
                    },
                    z: 0,
                    tooltip: { show: false }
                },
                {
                    type: 'bar',
                    data: data.map(item => item.value),
                    barWidth: 20,
                    barGap: '-100%',
                    itemStyle: {
                        color: new echarts.graphic.LinearGradient(
                            0, 0, 1, 0,
                            [
                                { offset: 0, color: 'rgba(34, 96, 242, 1)' },
                                { offset: 0.9259, color: 'rgba(34, 96, 242, 1)' },
                                { offset: 1, color: 'rgba(186, 207, 255, 1)' }
                            ]
                        ),
                        borderRadius: [2, 0, 0, 2],
                        borderRight: '8px solid transparent',
                        borderTop: '6px solid #eb8b0eff',
                        borderBottom: '6px solid #2260F2',
                    },
                    label: {
                        show: true,
                        position: 'insideLeft',
                        offset: [0, -23],
                        formatter: (params: any) => {
                            const item = data[params.dataIndex];
                            const formattedValue = item.value.toLocaleString();
                            const icon = rankIcons[item.rank - 1] || '🔹';

                            return `{icon|${icon}}{namePart|${item.name}}{valuePart|${formattedValue}}`;
                        },
                        rich: {
                            icon: {
                                width: 24,
                                height: 24,
                                fontSize: 14,
                                color:'black',
                                // verticalAlign: 'middle',
                                padding: [0, 5, 0,0], 
                                //align: 'center',
                                // 使用图标颜色映射
                                // color: (params: any) => {
                                //     const item = data[params.dataIndex];
                                //     return iconColors[item.rank - 1] || '#000000';
                                // },
                            },
                            namePart: {
                                align: 'left',
                                width: 120,
                                fontSize: 14,
                                color: 'rgba(0, 0, 0, 1)',
                                padding: [0, 10, 0, 5],
                                verticalAlign: 'middle'
                            },
                            valuePart: {
                                align: 'right',
                                width: 200,
                                fontSize: 14,
                                fontWeight: 'bold',
                                color: 'rgba(0, 0, 0, 1)',
                                padding: [0, 0, 0, 0],
                                verticalAlign: 'middle'
                            }
                        }
                    }
                }
            ]
        };

        chart.setOption(option);

        const handleResize = () => chart.resize();
        window.addEventListener('resize', handleResize);

        return () => {
            window.removeEventListener('resize', handleResize);
            chart.dispose();
        };
    }, []);

    return (
        <div
            ref={chartRef}
            style={{ width: '100%', height: '300px' }}
        />
    );
};

export default DepartmentRank;