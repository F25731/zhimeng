package control

func (s *Service) Dashboard() (map[string]interface{}, error) {
	summary, err := s.dashboardSummary()
	if err != nil {
		return nil, err
	}
	var daily []map[string]interface{}
	if err := s.db.Raw(`SELECT metric_date,COALESCE(sum(calls_total),0) calls_total,COALESCE(sum(users_total),0) users_total,COALESCE(sum(calls_success),0) calls_success,COALESCE(sum(calls_failed),0) calls_failed FROM site_daily_metrics WHERE metric_date>=current_date-interval '13 days' GROUP BY metric_date ORDER BY metric_date`).Scan(&daily).Error; err != nil {
		return nil, err
	}
	var abnormalSites []map[string]interface{}
	if err := s.db.Raw(`SELECT id,name,domain,status,last_heartbeat_at,last_error_message FROM sites WHERE deleted_at IS NULL AND status IN ('warning','offline','failed') ORDER BY updated_at DESC LIMIT 8`).Scan(&abnormalSites).Error; err != nil {
		return nil, err
	}
	var recentJobs []map[string]interface{}
	if err := s.db.Raw(`SELECT j.id,j.job_type,j.status,j.progress,j.current_step,j.created_at,s.name site_name,s.domain FROM deployment_jobs j LEFT JOIN sites s ON s.id=j.site_id ORDER BY j.created_at DESC LIMIT 8`).Scan(&recentJobs).Error; err != nil {
		return nil, err
	}
	summary["daily"] = daily
	summary["abnormal_sites"] = abnormalSites
	summary["recent_jobs"] = recentJobs
	return summary, nil
}
